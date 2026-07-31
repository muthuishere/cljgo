const http = require("http");

const port = Number(process.env.PORT || 8080);

const server = http.createServer((req, res) => {
  if (req.method === "GET" && req.url === "/") {
    res.writeHead(200, { "Content-Type": "text/plain" });
    res.end("hello\n");
    return;
  }
  if (req.method === "GET" && req.url === "/api/hello") {
    res.writeHead(200, { "Content-Type": "application/json" });
    res.end('{"msg":"hello from node"}');
    return;
  }
  res.writeHead(404, { "Content-Type": "text/plain" });
  res.end("not found\n");
});

server.listen(port);
