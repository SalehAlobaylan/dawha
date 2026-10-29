import { useQuery } from "@tanstack/react-query";
import { fetchChartLibrary } from "./api";

export function useChartWorkspace(fileId?: string) {
  const query = useQuery({
    queryKey: ["chart-library"], queryFn: fetchChartLibrary, enabled: Boolean(fileId), staleTime: 0,
    refetchInterval: (current) => current.state.data?.some((chart) => chart.fileId === fileId && (chart.readingStatus === "queued" || chart.readingStatus === "running" || chart.processingStatus === "queued" || chart.processingStatus === "running")) ? 3000 : false,
  });
  return { query, item: fileId ? query.data?.find((chart) => chart.fileId === fileId && chart.canReview) : undefined };
}

