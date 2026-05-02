// ─── openCrow API Config ───
// Provides runtime config to the browser (API base URL, version).

import type { NextApiRequest, NextApiResponse } from "next";

export default function handler(_req: NextApiRequest, res: NextApiResponse) {
  res.status(200).json({
    apiBaseUrl: process.env.API_BASE_URL || "http://localhost:8080",
    openCrowVersion: process.env.OPENCROW_VERSION || "dev",
  });
}
