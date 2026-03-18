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
import subprocess
import threading
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

# Tracks in-progress and completed background ingestion jobs
# { uri: { "status": "indexing"|"done"|"error", "namespace": str, "started_at": float, "finished_at": float|None, "error": str|None } }
_ingestion_jobs: Dict[str, Dict] = {}
_ingestion_lock = threading.Lock()


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


# ── Namespace helpers ─────────────────────────────────────────────────────────

def _derive_namespace(path: str) -> str:
    """Derive a deterministic namespace from a path.

    Priority:
    1. Git remote origin URL slug (stable across machines/users/paths)
    2. Git repo root directory name (stable within a machine)
    3. Directory name of the path itself (last resort)
    """
    dir_path = path if os.path.isdir(path) else str(Path(path).parent)

    # 1. Try git remote origin
    try:
        result = subprocess.run(
            ["git", "-C", dir_path, "remote", "get-url", "origin"],
            capture_output=True, text=True, timeout=5,
        )
        if result.returncode == 0:
            remote = result.stdout.strip()
            # Normalize: strip .git suffix, take last path segment
            # Handles both https://github.com/org/repo.git and git@github.com:org/repo.git
            slug = remote.rstrip("/")
            if slug.endswith(".git"):
                slug = slug[:-4]
            slug = slug.replace(":", "/").split("/")[-1]
            if slug:
                return slug
    except Exception:
        pass

    # 2. Try git repo root name
    try:
        result = subprocess.run(
            ["git", "-C", dir_path, "rev-parse", "--show-toplevel"],
            capture_output=True, text=True, timeout=5,
        )
        if result.returncode == 0:
            return Path(result.stdout.strip()).name
    except Exception:
        pass

    # 3. Directory name fallback
    return Path(dir_path).name


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
        kwargs: Dict[str, Any] = {"model": model, "messages": messages, "temperature": temperature, "max_tokens": max_tokens}
        if api_key:
            kwargs["api_key"] = api_key
        if api_base:
            kwargs["api_base"] = api_base
        resp = litellm.completion(**kwargs)
        return resp.choices[0].message.content or ""


def _query_sync(
    user_query: str,
    top_k: int = 5,
    temperature: float = 0.7,
    max_tokens: int = 2048,
    score_threshold: float = 0.2,
    system_prompt: Optional[str] = None,
    namespace: Optional[str] = None,
) -> Dict[str, Any]:
    t0 = time.perf_counter()

    t_search = time.perf_counter()
    search_results = _search_sync(user_query, top_k=top_k, score_threshold=score_threshold, target_uri=namespace)
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
        namespace: str = "",
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
            namespace:        Optional viking:// URI to scope the search to a specific knowledge base
                              (e.g. "viking://my-project"). Use list_namespaces to discover available namespaces.
        """
        result = await asyncio.to_thread(
            _query_sync,
            user_query=question,
            top_k=top_k,
            temperature=temperature,
            max_tokens=max_tokens,
            score_threshold=score_threshold,
            system_prompt=system_prompt or None,
            namespace=namespace or None,
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
    async def remove_resource(uri: str) -> str:
        """
        Remove a resource or entire namespace from the OpenViking knowledge base.

        Deletes the resource and all its indexed data recursively. Use list_namespaces
        to find the URI to remove.

        Args:
            uri: The viking:// URI to remove (e.g. "viking://resources/my-project").
                 Supports any URI depth — removes everything under it recursively.
        """
        def _remove():
            with open(_config_path) as f:
                cfg = json.load(f)
            config = OpenVikingConfig.from_dict(cfg)
            client = ov.SyncOpenViking(path=_data_path, config=config)
            try:
                client.initialize()
                client.rm(uri, recursive=True)
                return f"Removed: {uri}"
            finally:
                client.close()

        return await asyncio.to_thread(_remove)

    @mcp.tool()
    async def list_namespaces() -> str:
        """
        List all available namespaces (ingested knowledge bases) in OpenViking.

        Returns the viking:// URIs you can pass as the `namespace` parameter to
        `query` or `search` to scope results to a specific knowledge base.
        """
        def _list():
            data_dir = Path(_data_path)
            resources_dir = data_dir / "viking" / "default" / "resources"
            if not resources_dir.exists():
                return []
            return sorted(
                f"viking://resources/{p.name}"
                for p in resources_dir.iterdir()
                if p.is_dir() and not p.name.startswith(".")
            )

        namespaces = await asyncio.to_thread(_list)
        if not namespaces:
            return "No namespaces found. Use add_resource to ingest documents first."
        return "Available namespaces:\n" + "\n".join(f"  {ns}" for ns in namespaces)

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
            target_uri:       Optional viking:// URI to scope the search to a specific namespace.
                              Use list_namespaces to discover available namespaces.
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
    async def add_resource(resource_path: str, namespace: str = "") -> str:
        """
        Add a document, file, directory, or URL to the OpenViking knowledge base.

        The resource will be parsed, chunked, and indexed for future search/query.
        Supported: PDF, Markdown, plain text, HTML, images, code files, and URLs.

        Args:
            resource_path: Local file/directory path, or a URL to ingest.
            namespace:     Logical name to scope this resource under (e.g. "devexp-toolkit",
                           "my-project"). Stored as viking://resources/<namespace>.
                           If omitted, auto-derived from the git remote origin URL
                           (stable across machines and users), falling back to the
                           git repo root name, then the directory name.
        """
        config_path = _config_path
        data_path = _data_path

        def _add():
            with open(config_path) as f:
                cfg = json.load(f)
            config = OpenVikingConfig.from_dict(cfg)
            client = ov.SyncOpenViking(path=data_path, config=config)
            client.initialize()
            path = resource_path
            if not path.startswith("http"):
                resolved = Path(path).expanduser()
                if not resolved.exists():
                    client.close()
                    return f"Error: path not found: {resolved}"
                path = str(resolved)
            resolved_ns = namespace or _derive_namespace(path)
            ns_uri = f"viking://resources/{resolved_ns}"
            ns_dir = Path(data_path) / "viking" / "default" / "resources" / resolved_ns
            if ns_dir.exists():
                result = client.add_resource(path=path, parent=ns_uri)
            else:
                result = client.add_resource(path=path, to=ns_uri)
            if result:
                result["_namespace"] = resolved_ns
            if result and "root_uri" in result:
                root_uri = result["root_uri"]
                job_ns = result.get("_namespace", resolved_ns)
                with _ingestion_lock:
                    _ingestion_jobs[root_uri] = {
                        "status": "indexing",
                        "namespace": job_ns,
                        "started_at": time.time(),
                        "finished_at": None,
                        "error": None,
                    }

                # Run wait_processed in a background thread so the MCP response
                # returns immediately — avoids HTTP timeout on large files/dirs.
                def _bg_finish(uri=root_uri):
                    try:
                        client.wait_processed(timeout=600)
                        with _ingestion_lock:
                            _ingestion_jobs[uri]["status"] = "done"
                            _ingestion_jobs[uri]["finished_at"] = time.time()
                        logger.info(f"Indexing complete: {uri}")
                    except Exception as e:
                        with _ingestion_lock:
                            _ingestion_jobs[uri]["status"] = "error"
                            _ingestion_jobs[uri]["finished_at"] = time.time()
                            _ingestion_jobs[uri]["error"] = str(e)
                        logger.warning(f"wait_processed error for {uri}: {e}")
                    finally:
                        client.close()

                threading.Thread(target=_bg_finish, daemon=True).start()
                return (
                    f"Ingestion started: {root_uri} "
                    f"(namespace: {job_ns})\n"
                    "Indexing in background — call check_ingestion() to monitor progress."
                )
            elif result and result.get("status") == "error":
                client.close()
                errors = result.get("errors", [])[:3]
                return "Partial error:\n" + "\n".join(f"  - {e}" for e in errors)
            client.close()
            return "Failed to add resource."

        return await asyncio.to_thread(_add)

    @mcp.tool()
    async def check_ingestion(uri: str = "") -> str:
        """
        Check the status of background ingestion jobs started by add_resource.

        Args:
            uri: Optional viking:// URI returned by add_resource to check a specific job.
                 If omitted, returns the status of all known jobs.
        """
        with _ingestion_lock:
            if not _ingestion_jobs:
                return "No ingestion jobs recorded in this server session."

            jobs = (
                {uri: _ingestion_jobs[uri]} if uri and uri in _ingestion_jobs
                else dict(_ingestion_jobs)
            )
            if uri and uri not in _ingestion_jobs:
                return f"No job found for URI: {uri}"

        lines = []
        for job_uri, info in sorted(jobs.items(), key=lambda x: x[1]["started_at"], reverse=True):
            status = info["status"]
            ns = info["namespace"]
            elapsed = (info["finished_at"] or time.time()) - info["started_at"]
            icon = {"indexing": "⏳", "done": "✅", "error": "❌"}.get(status, "?")
            line = f"{icon} [{status}] {job_uri}  (ns: {ns}, elapsed: {elapsed:.0f}s)"
            if info["error"]:
                line += f"\n   error: {info['error']}"
            lines.append(line)

        return "\n".join(lines)

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
