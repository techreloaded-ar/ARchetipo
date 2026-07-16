export interface Trip {
  id: string;
  title: string;
  distanceKm: number;
}

export type TripStatus = "Draft" | "ReadyForReview" | "Published";
