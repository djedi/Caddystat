const http = require('http');
const fs = require('fs');
const path = require('path');
const httpProxy = require('http-proxy');

const PORT = 8405;
const API_TARGET = 'http://localhost:8404';
const STATIC_DIR = path.join(__dirname, '_site');

const proxy = httpProxy.createProxyServer({ target: API_TARGET });

proxy.on('error', (err, req, res) => {
  console.error('Proxy error:', err.message);
  if (res.headersSent) {
    res.end();
    return;
  }
  res.writeHead(502, { 'Content-Type': 'text/plain' });
  res.end('API server unavailable');
});

const MIME_TYPES = {
  '.html': 'text/html',
  '.css': 'text/css',
  '.js': 'application/javascript',
  '.json': 'application/json',
  '.png': 'image/png',
  '.jpg': 'image/jpeg',
  '.svg': 'image/svg+xml',
  '.ico': 'image/x-icon',
};

const server = http.createServer((req, res) => {
  // Proxy /api requests to Go server
  if (req.url.startsWith('/api/')) {
    return proxy.web(req, res);
  }

  // Parse URL and sanitize path to prevent directory traversal
  const urlPath = new URL(req.url, `http://localhost:${PORT}`).pathname;
  const safePath = path.normalize(urlPath).replace(/^(\.\.[\/\\])+/, '');
  let filePath = path.join(STATIC_DIR, safePath === '/' ? 'index.html' : safePath);

  // Ensure the resolved path is within STATIC_DIR
  if (!filePath.startsWith(STATIC_DIR)) {
    res.writeHead(403);
    res.end('Forbidden');
    return;
  }

  const ext = path.extname(filePath);

  fs.readFile(filePath, (err, data) => {
    if (err) {
      // Fallback to index.html for SPA routing
      fs.readFile(path.join(STATIC_DIR, 'index.html'), (err2, data2) => {
        if (err2) {
          res.writeHead(404);
          res.end('Not found');
          return;
        }
        res.writeHead(200, {
          'Content-Type': 'text/html',
          'Cache-Control': 'no-cache'
        });
        res.end(data2);
      });
      return;
    }
    res.writeHead(200, {
      'Content-Type': MIME_TYPES[ext] || 'application/octet-stream',
      'Cache-Control': 'no-cache'
    });
    res.end(data);
  });
});

// Handle WebSocket upgrades (if any API endpoints use WebSocket)
server.on('upgrade', (req, socket, head) => {
  if (req.url.startsWith('/api/')) {
    proxy.ws(req, socket, head);
  }
});

server.listen(PORT, () => {
  console.log(`Dev server running at http://localhost:${PORT}`);
  console.log(`Proxying /api/* to ${API_TARGET}`);
});
