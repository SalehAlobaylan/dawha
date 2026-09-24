import { demoDashboard } from "../data/demo";
import type { DashboardData } from "../types";

const apiBaseUrl = import.meta.env.VITE_API_URL ?? "";

export async function fetchDashboard(): Promise<DashboardData> {
  if (!apiBaseUrl) {
    return demoDashboard;
  }

  try {
    const response = await fetch(`${apiBaseUrl}/api/v1/dashboard`, { signal: AbortSignal.timeout(2500) });
    if (!response.ok) {
      throw new Error(`Dashboard request failed with ${response.status}`);
    }
    return (await response.json()) as DashboardData;
  } catch {
    return { ...demoDashboard, mode: "demo" };
  }
}
