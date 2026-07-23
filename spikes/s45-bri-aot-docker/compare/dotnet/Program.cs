using Microsoft.AspNetCore.Http;

var builder = WebApplication.CreateBuilder(args);
var app = builder.Build();

var port = Environment.GetEnvironmentVariable("PORT") ?? "8080";
app.Urls.Add($"http://0.0.0.0:{port}");

app.MapGet("/", () =>
    Results.Text("hello\n", "text/plain"));

app.MapGet("/api/hello", () =>
    // Exact bytes, hand-built JSON string (not object serialization).
    Results.Text("{\"msg\":\"hello from dotnet\"}", "application/json"));

app.Run();
