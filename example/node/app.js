// app.js
const http = require('http');
const port = process.argv[2] || 3000;

const server = http.createServer((req, res) => {
    res.statusCode = 200;
    res.setHeader('Content-Type', 'text/plain');
    res.end(`Hello from Node.js on port ${port}!\n`);
});

server.listen(port, '127.0.0.1', () => {
    console.log(`Server running at http://127.0.0.1:${port}/`);
});

console.log('App is starting...');

// Keep the process alive for a while for testing
setTimeout(() => {
    console.log('App is still running...');
}, 60000); // 1 minute