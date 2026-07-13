import { Router } from "express";

export const trips = Router();
trips.get("/", (_request, response) => response.json([]));
