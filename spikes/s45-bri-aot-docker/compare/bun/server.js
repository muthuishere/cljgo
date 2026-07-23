const port = Number(process.env.PORT || 8080);

Bun.serve({
  port,
  fetch(req) {
    const url = new URL(req.url);
    if (url.pathname === "/") {
      return new Response("hello\n", {
        status: 200,
        headers: { "Content-Type": "text/plain" },
      });
    }
    if (url.pathname === "/api/hello") {
      return new Response('{"msg":"hello from bun"}', {
        status: 200,
        headers: { "Content-Type": "application/json" },
      });
    }
    return new Response("not found\n", { status: 404 });
  },
});
