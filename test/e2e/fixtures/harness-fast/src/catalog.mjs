import { createServer } from 'node:http';

export function catalogServer() {
  const trips = [{ id: 1, title: 'Alpi in moto', vehicle: 'motorcycle' }];
  return createServer(async (request, response) => {
    response.setHeader('content-type', 'application/json');
    const url = new URL(request.url, 'http://localhost');
    if (url.pathname !== '/trips') { response.writeHead(404).end('{}'); return; }
    if (request.method === 'GET') { response.end(JSON.stringify(trips)); return; }
    if (request.method !== 'POST') { response.writeHead(405).end('{}'); return; }
    try {
      let body = '';
      for await (const chunk of request) body += chunk;
      const input = JSON.parse(body);
      const vehicle = input.vehicle ?? 'motorcycle';
      if (typeof input.title !== 'string' || !input.title.trim() || vehicle !== 'motorcycle') {
        response.writeHead(400).end('{}'); return;
      }
      const trip = { id: trips.length + 1, title: input.title.trim(), vehicle };
      trips.push(trip);
      response.writeHead(201).end(JSON.stringify(trip));
    } catch { response.writeHead(400).end('{}'); }
  });
}
