// KYC 上传组件 —— OSS direct upload via presigned PUT
// 兼容图片 / PDF；上传完毕回调 object_url
import * as React from "react";
import { useTranslation } from "react-i18next";
import axios from "axios";
import { presignKycUpload, type KycUploadKind, type KycPresignResponse } from "@/api";

export interface KycUploaderProps {
  kind: KycUploadKind;
  label: string;
  /** 当前已上传的 object_url（草稿恢复时为空，上传成功后回填） */
  value?: string;
  /** 上传完成回调 */
  onChange: (objectUrl: string) => void;
  /** byte cap；默认 5 MB */
  maxBytes?: number;
  /** 接受的 mime；默认 image/* + application/pdf */
  accept?: string;
  required?: boolean;
}

const DEFAULT_MAX = 5 * 1024 * 1024;

export function KycUploader(props: KycUploaderProps): JSX.Element {
  const { t } = useTranslation();
  const [progress, setProgress] = React.useState(0);
  const [error, setError] = React.useState<string | null>(null);
  const [busy, setBusy] = React.useState(false);
  const inputRef = React.useRef<HTMLInputElement>(null);
  const maxBytes = props.maxBytes ?? DEFAULT_MAX;
  const accept = props.accept ?? "image/jpeg,image/png,application/pdf,video/mp4";

  const onPick = React.useCallback(
    async (e: React.ChangeEvent<HTMLInputElement>) => {
      const file = e.target.files?.[0];
      if (!file) return;
      if (file.size > maxBytes) {
        setError(t("apply.kyc.size_limit", { mb: Math.round(maxBytes / 1024 / 1024) }));
        return;
      }
      setError(null);
      setBusy(true);
      setProgress(0);
      try {
        const presign: KycPresignResponse = await presignKycUpload({
          kind: props.kind,
          content_type: file.type || "application/octet-stream",
          size: file.size,
        });
        await axios.put(presign.upload_url, file, {
          headers: { "Content-Type": file.type, ...presign.required_headers },
          onUploadProgress: (evt) => {
            if (evt.total) setProgress(Math.round((evt.loaded / evt.total) * 100));
          },
        });
        props.onChange(presign.object_url);
        setProgress(100);
      } catch (err: unknown) {
        const msg = err instanceof Error ? err.message : t("errors.network");
        setError(msg);
      } finally {
        setBusy(false);
        if (inputRef.current) inputRef.current.value = "";
      }
    },
    [props, maxBytes, t],
  );

  const id = `kyc-${props.kind}`;

  return (
    <div style={{ marginBottom: 16 }}>
      <label htmlFor={id} style={{ display: "block", color: "#cbd5e1", marginBottom: 6 }}>
        {props.label} {props.required ? <span style={{ color: "#f87171" }}>*</span> : null}
      </label>
      <input
        ref={inputRef}
        id={id}
        type="file"
        accept={accept}
        onChange={onPick}
        disabled={busy}
        aria-describedby={`${id}-hint`}
      />
      <div id={`${id}-hint`} style={{ fontSize: 12, color: "#9ca3af", marginTop: 4 }}>
        {t("apply.kyc.size_limit", { mb: Math.round(maxBytes / 1024 / 1024) })}
      </div>
      {busy ? (
        <div style={{ marginTop: 6, color: "#9ca3af" }} role="status">
          {t("apply.kyc.uploading")} {progress}%
        </div>
      ) : props.value ? (
        <div style={{ marginTop: 6, color: "#22c55e" }} role="status">
          ✓ {t("apply.kyc.uploaded")}
        </div>
      ) : null}
      {error ? (
        <div role="alert" style={{ marginTop: 6, color: "#f87171", fontSize: 12 }}>
          {error}
        </div>
      ) : null}
    </div>
  );
}
