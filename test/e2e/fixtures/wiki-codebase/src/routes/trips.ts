import { Router } from "express";
import type { TripStatus } from "../domain/trip";

export const trips = Router();
let status: TripStatus = "Draft";

trips.get("/", (_request, response) => response.json([]));
trips.post("/:id/publish", (_request, response) => {
  if (status !== "Draft") {
    return response.status(409).json({ error: "Only drafts can be published" });
  }
  status = "Published";
  return response.json({ status });
});
