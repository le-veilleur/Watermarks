// Serveur statique minimal pour distroless — zéro dépendance externe.
// SPA fallback : toute route inconnue renvoie index.html pour que React Router fonctionne.
import { createServer } from "node:http";
import { createReadStream, existsSync, statSync } from "node:fs";
import { join, extname } from "node:path";
import { fileURLToPath } from "node:url";

const PORT = 5173;
const DIST = join(fileURLToPath(import.meta.url), "..", "dist");

const mime = {
  ".html": "text/html; charset=utf-8",
  ".js":   "application/javascript",
  ".css":  "text/css",
  ".svg":  "image/svg+xml",
  ".png":  "image/png",
  ".jpg":  "image/jpeg",
  ".ico":  "image/x-icon",
  ".json": "application/json",
  ".woff2": "font/woff2",
};

createServer((req, res) => {
  let file = join(DIST, req.url === "/" ? "/index.html" : req.url);

  // SPA fallback — fichier absent ou répertoire → index.html
  if (!existsSync(file) || statSync(file).isDirectory()) {
    file = join(DIST, "index.html");
  }

  res.writeHead(200, { "Content-Type": mime[extname(file)] ?? "application/octet-stream" });
  createReadStream(file).pipe(res);
}).listen(PORT, "0.0.0.0", () => {
  console.log(JSON.stringify({ level: "info", service: "front", addr: `:${PORT}`, message: "démarrage" }));
});
