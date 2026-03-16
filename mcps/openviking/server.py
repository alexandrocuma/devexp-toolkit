#!/usr/bin/env python3
"""
OpenViking MCP Server — Expose RAG query capabilities through Model Context Protocol

Tools:
  query        Full RAG pipeline — semantic search + LLM answer generation
  search       Semantic search only, returns matching documents with scores
  add_resource Add files, directories, or URLs to the knowledge base

Usage:
  python server.py
  python server.py --config ./ov.conf --data ./data --port 2033
  python server.py --transport stdio

Environment variables:
  OV_CONFIG    Path to config file  (default: ./ov.conf)
  OV_DATA      Path to data dir     (default: ./data)
  OV_PORT      Listen port          (default: 2033)
  OV_DEBUG     Enable debug logging (set to 1)
"""

import argparse
import asyncio
import json
import logging
import os
import time
from pathlib import Path
from typing import Any, Dict, List, Optional

import openviking as ov
from mcp.server.fastmcp import FastMCP
from openviking_cli.utils.config.open_viking_config import OpenVikingConfig

logging.basicConfig(level=logging.INFO)
logger = logging.getLogger("openviking-mcp")

# ── Global state ──────────────────────────────────────────────────────────────
_config_path: str = os.getenv("OV_CONFIG", "./ov.conf")
_data_path: str = os.getenv("OV_DATA", "./data")
_client: Optional[ov.SyncOpenViking] = None
_config_dict: Optional[Dict] = None


def _get_client() -> ov.SyncOpenViking:
    global _client, _config_dict
    if _client is None:
        with open(_config_path) as f:
            _config_dict = json.load(f)
        config = OpenVikingConfig.from_dict(_config_dict)
        _client = ov.SyncOpenViking(path=_data_path, config=config)
        _client.initialize()
    return _client


def _get_config() -> Dict:
    global _config_dict
    if _config_dict is None:
        with open(_config_path) as f:
            _config_dict = json.load(f)
    return _config_dict


# ── RAG helpers ───────────────────────────────────────────────────────────────

def _search_sync(
    query: str,
    top_k: int = 5,
    score_threshold: float = 0.2,
    target_uri: Optional[str] = None,
) -> List[Dict[str, Any]]:
    client = _get_client()
    results = client.search(query, target_uri=target_uri, score_threshold=score_threshold)

    output = []
    for resource in (results.resources[:top_k] + results.memories[:top_k]):
        try:
            content = client.read(resource.uri)
        except Exception as e:
            if "is a directory" in str(e):
                try:
                    content = f"[Directory] {client.abstract(resource.uri)}"
                except Exception:
                    content = "[Directory — abstract unavailable]"
            else:
                continue
        output.append({"uri": resource.uri, "score": resource.score, "content": content})

    return output


def _call_llm(messages: List[Dict], temperature: float, max_tokens: int) -> str:
    cfg = _get_config().get("vlm", {})
    provider = cfg.get("provider", "litellm")
    api_key = cfg.get("api_key", "")
    api_base = cfg.get("api_base")
    model = cfg.get("model", "gpt-4o")

    if provider == "volcengine":
        from openai import OpenAI
        client = OpenAI(api_key=api_key, base_url=api_base or "https://ark.cn-beijing.volces.com/api/v3")
        resp = client.chat.completions.create(model=model, messages=messages, temperature=temperature, max_tokens=max_tokens)
        return resp.choices[0].message.content or ""

    elif provider == "openai":
        from openai import OpenAI
        client = OpenAI(api_key=api_key, base_url=api_base or "https://api.openai.com/v1")
        resp = client.chat.completions.create(model=model, messages=messages, temperature=temperature, max_tokens=max_tokens)
        return resp.choices[0].message.content or ""

    else:  # litellm (default — supports Anthropic, DeepSeek, Gemini, Ollama, etc.)
        import litellm
        if api_key:
            litellm.api_key = api_key
        if api_base:
            litellm.api_base = api_base
        resp = litellm.completion(model=model, messages=messages, temperature=temperature, max_tokens=max_tokens)
        return resp.choices[0].message.content or ""


def _query_sync(
    user_query: str,
    top_k: int = 5,
    temperature: float = 0.7,
    max_tokens: int = 2048,
    score_threshold: float = 0.2,
    system_prompt: Optional[str] = None,
) -> Dict[str, Any]:
    t0 = time.perf_counter()

    t_search = time.perf_counter()
    search_results = _search_sync(user_query, top_k=top_k, score_threshold=score_threshold)
    search_time = time.perf_counter() - t_search

    if search_results:
        context_text = (
            "Answer pivoting to the following context:\n<context>\n"
            + "\n\n".join(
                f"[Source {i+1}] (relevance: {r['score']:.4f})\n{r['content']}"
                for i, r in enumerate(search_results)
            )
            + "\n</context>"
        )
    else:
        context_text = "No relevant information found — answer from general knowledge."

    messages = [
        {
            "role": "system",
            "content": system_prompt or "Answer questions concisely in plain text.",
        },
        {
            "role": "user",
            "content": f"{context_text}\n\nQuestion: {user_query}",
        },
    ]

    t_llm = time.perf_counter()
    answer = _call_llm(messages, temperature=temperature, max_tokens=max_tokens)
    llm_time = time.perf_counter() - t_llm

    return {
        "answer": answer,
        "context": search_results,
        "timings": {
            "search_time": search_time,
            "llm_time": llm_time,
            "total_time": time.perf_counter() - t0,
        },
    }


# ── MCP server ────────────────────────────────────────────────────────────────

def create_server(host: str = "0.0.0.0", port: int = 2033) -> FastMCP:
    mcp = FastMCP(
        name="openviking-mcp",
        instructions=(
            "OpenViking MCP provides RAG capabilities over a tiered context database. "
            "Use 'query' for full RAG answers, 'search' for semantic search only, "
            "and 'add_resource' to ingest new documents or URLs."
        ),
        host=host,
        port=port,
        stateless_http=True,
        json_response=True,
    )

    @mcp.tool()
    async def query(
        question: str,
        top_k: int = 5,
        temperature: float = 0.7,
        max_tokens: int = 2048,
        score_threshold: float = 0.2,
        system_prompt: str = "",
    ) -> str:
        """
        Ask a question and get an answer using RAG (Retrieval-Augmented Generation).

        Searches the OpenViking knowledge base for relevant context, then generates
        an answer using an LLM with the retrieved context as grounding.

        Args:
            question:         The question to ask.
            top_k:            Number of search results to use as context (default: 5).
            temperature:      LLM sampling temperature 0.0–1.0 (default: 0.7).
            max_tokens:       Maximum tokens in the response (default: 2048).
            score_threshold:  Minimum relevance score 0.0–1.0 (default: 0.2).
            system_prompt:    Optional system prompt to guide the LLM response style.
        """
        result = await asyncio.to_thread(
            _query_sync,
            user_query=question,
            top_k=top_k,
            temperature=temperature,
            max_tokens=max_tokens,
            score_threshold=score_threshold,
            system_prompt=system_prompt or None,
        )

        output = result["answer"]

        if result["context"]:
            output += "\n\n---\nSources:\n"
            for i, ctx in enumerate(result["context"], 1):
                filename = ctx["uri"].split("/")[-1] or ctx["uri"]
                output += f"  {i}. {filename} (relevance: {ctx['score']:.4f})\n"

        t = result["timings"]
        output += (
            f"\n[search: {t['search_time']:.2f}s, "
            f"llm: {t['llm_time']:.2f}s, "
            f"total: {t['total_time']:.2f}s]"
        )
        return output

    @mcp.tool()
    async def search(
        query: str,
        top_k: int = 5,
        score_threshold: float = 0.2,
        target_uri: str = "",
    ) -> str:
        """
        Semantic search over the OpenViking knowledge base (no LLM generation).

        Returns matching documents with relevance scores. Use this when you need
        to find relevant documents without generating an answer.

        Args:
            query:            The search query.
            top_k:            Number of results to return (default: 5).
            score_threshold:  Minimum relevance score 0.0–1.0 (default: 0.2).
            target_uri:       Optional viking:// URI to scope the search.
        """
        results = await asyncio.to_thread(
            _search_sync,
            query=query,
            top_k=top_k,
            score_threshold=score_threshold,
            target_uri=target_uri or None,
        )

        if not results:
            return "No relevant results found."

        parts = []
        for i, r in enumerate(results, 1):
            preview = r["content"][:500] + "..." if len(r["content"]) > 500 else r["content"]
            parts.append(f"[{i}] {r['uri']} (score: {r['score']:.4f})\n{preview}")

        return f"Found {len(results)} results:\n\n" + "\n\n".join(parts)

    @mcp.tool()
    async def add_resource(resource_path: str) -> str:
        """
        Add a document, file, directory, or URL to the OpenViking knowledge base.

        The resource will be parsed, chunked, and indexed for future search/query.
        Supported: PDF, Markdown, plain text, HTML, images, code files, and URLs.

        Args:
            resource_path: Local file/directory path, or a URL to ingest.
        """
        config_path = _config_path
        data_path = _data_path

        def _add():
            with open(config_path) as f:
                cfg = json.load(f)
            config = OpenVikingConfig.from_dict(cfg)
            client = ov.SyncOpenViking(path=data_path, config=config)
            try:
                client.initialize()
                path = resource_path
                if not path.startswith("http"):
                    resolved = Path(path).expanduser()
                    if not resolved.exists():
                        return f"Error: path not found: {resolved}"
                    path = str(resolved)
                result = client.add_resource(path=path)
                if result and "root_uri" in result:
                    client.wait_processed(timeout=300)
                    return f"Added and indexed: {result['root_uri']}"
                elif result and result.get("status") == "error":
                    errors = result.get("errors", [])[:3]
                    return "Partial error:\n" + "\n".join(f"  - {e}" for e in errors)
                return "Failed to add resource."
            finally:
                client.close()

        return await asyncio.to_thread(_add)

    @mcp.resource("openviking://status")
    def server_status() -> str:
        """Current server status and config paths."""
        return json.dumps({"config": _config_path, "data": _data_path, "status": "running"}, indent=2)

    return mcp


# ── Entry point ───────────────────────────────────────────────────────────────

def parse_args():
    p = argparse.ArgumentParser(description="OpenViking MCP Server")
    p.add_argument("--config", default=os.getenv("OV_CONFIG", "./ov.conf"))
    p.add_argument("--data", default=os.getenv("OV_DATA", "./data"))
    p.add_argument("--host", default="0.0.0.0")
    p.add_argument("--port", type=int, default=int(os.getenv("OV_PORT", "2033")))
    p.add_argument("--transport", choices=["streamable-http", "stdio"], default="streamable-http")
    return p.parse_args()


def main():
    args = parse_args()

    global _config_path, _data_path
    _config_path = args.config
    _data_path = args.data

    if os.getenv("OV_DEBUG") == "1":
        logging.getLogger().setLevel(logging.DEBUG)

    logger.info("OpenViking MCP Server starting")
    logger.info(f"  config:    {_config_path}")
    logger.info(f"  data:      {_data_path}")
    logger.info(f"  transport: {args.transport}")

    mcp = create_server(host=args.host, port=args.port)

    if args.transport == "streamable-http":
        logger.info(f"  endpoint:  http://{args.host}:{args.port}/mcp")
        mcp.run(transport="streamable-http")
    else:
        mcp.run(transport="stdio")


if __name__ == "__main__":
    main()
