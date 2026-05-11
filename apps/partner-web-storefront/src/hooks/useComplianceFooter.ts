// 复合 hook：合规 footer 数据 + ICP feature flag fallback
// frontend §11.5 / Compliance CRIT-1：9 个备案号缺一不可
import { useQuery } from "@tanstack/react-query";
import { fetchComplianceFooter } from "@/api";
import type { ComplianceFooterDTO } from "@/api";

export function useComplianceFooter() {
  return useQuery<ComplianceFooterDTO>({
    queryKey: ["public", "compliance-footer"],
    queryFn: fetchComplianceFooter,
    staleTime: 5 * 60_000,
    retry: 1,
  });
}
