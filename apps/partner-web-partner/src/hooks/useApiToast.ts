// useApiToast —— 把 ApiException 映射到 toast 文案；统一错误展示
import { useCallback } from "react";
import { useTranslation } from "react-i18next";
import { Toast } from "@douyinfe/semi-ui";
import { ApiException } from "@/api/types";
import { mapApiError } from "@/api/error-mapping";

export function useApiToast(): {
  showError: (e: unknown) => void;
  showSuccess: (msg: string) => void;
  showInfo: (msg: string) => void;
} {
  const { t } = useTranslation();
  const showError = useCallback(
    (e: unknown) => {
      if (e instanceof ApiException) {
        const spec = mapApiError({
          code: e.code,
          trace_id: e.traceId,
        });
        const text = t(spec.i18nKey, { defaultValue: e.message });
        if (spec.severity === "warning") Toast.warning({ content: text });
        else if (spec.severity === "info") Toast.info({ content: text });
        else Toast.error({ content: text });
        return;
      }
      const msg = e instanceof Error ? e.message : String(e);
      Toast.error({ content: msg });
    },
    [t],
  );
  const showSuccess = useCallback((msg: string) => Toast.success({ content: msg }), []);
  const showInfo = useCallback((msg: string) => Toast.info({ content: msg }), []);
  return { showError, showSuccess, showInfo };
}
