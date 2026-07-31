import os

from fastapi import FastAPI
from fastapi.responses import Response

app = FastAPI()


@app.get("/")
def root() -> Response:
    return Response(content="hello\n", media_type="text/plain")


@app.get("/api/hello")
def hello() -> Response:
    # Hand-built body: exact bytes, no reordering / whitespace.
    return Response(
        content='{"msg":"hello from fastapi"}',
        media_type="application/json",
    )


if __name__ == "__main__":
    import uvicorn

    port = int(os.environ.get("PORT", "8080"))
    # workers=1 for a fair single-process baseline vs the other runtimes.
    uvicorn.run(app, host="0.0.0.0", port=port, workers=1, log_level="warning")
