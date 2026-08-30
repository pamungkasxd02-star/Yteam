#!/usr/bin/env python3
"""Run the LocalSolver browser-observation API on a local interface."""

from __future__ import annotations

import argparse
import os
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
SRC = ROOT / "src"
if str(SRC) not in sys.path:
    sys.path.insert(0, str(SRC))


def main() -> int:
    try:
        from fastapi import FastAPI, Header, HTTPException
        from pydantic import BaseModel, Field
        import uvicorn
        from local_solver.service import LocalSolverService
    except ImportError as error:
        print(f"LocalSolver requires requirements.txt dependencies: {error}", file=sys.stderr)
        return 2

    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--host", default=os.environ.get("LOCALSOLVER_HOST", "127.0.0.1"))
    parser.add_argument("--port", type=int, default=int(os.environ.get("LOCALSOLVER_PORT", "8001")))
    parser.add_argument("--workers", type=int, default=int(os.environ.get("LOCALSOLVER_WORKERS", "2")))
    args = parser.parse_args()
    if args.host not in {"127.0.0.1", "localhost", "::1"} and not os.environ.get("LOCALSOLVER_API_KEY"):
        print("Refusing non-local LocalSolver binding without LOCALSOLVER_API_KEY", file=sys.stderr)
        return 2
    service = LocalSolverService(ROOT, args.workers)
    app = FastAPI(title="LocalSolver", version="1.0.0")

    class ObserveRequest(BaseModel):
        url: str
        headless: bool = True
        timeout_ms: int = Field(default=12_000, ge=3_000, le=30_000)
        rate: float = Field(default=1.0, ge=0.1, le=5.0)

    def authorize(key: str = "") -> None:
        expected = os.environ.get("LOCALSOLVER_API_KEY", "")
        if expected and key != expected:
            raise HTTPException(status_code=401, detail="invalid LocalSolver API key")

    @app.get("/health")
    def health() -> dict[str, object]:
        return {"ok": True, "service": "localsolver", "policy": "local allowlisted browser observation"}

    @app.post("/observe", status_code=202)
    def observe(request: ObserveRequest, x_localsolver_key: str = Header(default="")) -> dict[str, object]:
        authorize(x_localsolver_key)
        try:
            return service.submit(request.url, request.model_dump())
        except PermissionError as error:
            raise HTTPException(status_code=403, detail=str(error)) from error

    @app.get("/result")
    def result(id: str, x_localsolver_key: str = Header(default="")) -> dict[str, object]:
        authorize(x_localsolver_key)
        value = service.result(id)
        if value is None:
            raise HTTPException(status_code=404, detail="task not found")
        return value

    @app.get("/tasks")
    def tasks(x_localsolver_key: str = Header(default="")) -> dict[str, object]:
        authorize(x_localsolver_key)
        return {"tasks": service.queue.list()}

    try:
        uvicorn.run(app, host=args.host, port=args.port, log_level="info")
    finally:
        service.close()
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
