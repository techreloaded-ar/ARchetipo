import express from "express";
import { trips } from "./routes/trips";

const app = express();
app.use("/api/trips", trips);
app.listen(3000);
