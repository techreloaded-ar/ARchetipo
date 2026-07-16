import express from "express";

const app = express();
app.get("/api/routes", (_request, response) => response.json([]));
app.listen(3000);
