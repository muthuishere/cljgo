const port = Number(Deno.env.get("PORT") ?? "8080");

Deno.serve({ port }, (req: Request) => {
  const url = new URL(req.url);
  if (url.pathname === "/") {
    return new Response("hello\n", {
      status: 200,
      headers: { "Content-Type": "text/plain" },
    });
  }
  if (url.pathname === "/api/hello") {
    return new Response('{"msg":"hello from deno"}', {
      status: 200,
      headers: { "Content-Type": "application/json" },
    });
  }
  return new Response("not found\n", { status: 404 });
});
