// 12377 内容安全列表 + 详情 + 重试 + 一键派发（COMP-CRIT-2）
import { useState } from "react";
import { Link, useParams } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Banner, Button, Card, Descriptions, Input, Modal, Spin, Table, Tag } from "@douyinfe/semi-ui";
import { useTranslation } from "react-i18next";
import { Page } from "@/components/Page";
import { Field } from "@/components/Field";
import { listContentReports, getContentReport, retryContentReport, dispatchContentReports, type ContentSafetyReport } from "@/api/admin";
import { useApiToast } from "@/hooks/useApiToast";

export function ContentSafetyReports(): JSX.Element {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const { showError, showSuccess } = useApiToast();
  const [page, setPage] = useState(1);
  const [open, setOpen] = useState(false);
  const [batch, setBatch] = useState<string>("50");

  const { data, isLoading } = useQuery({
    queryKey: ["admin", "content-reports", page],
    queryFn: () => listContentReports({ page, limit: 50 }),
    placeholderData: (prev) => prev,
  });

  const dispatchMut = useMutation({
    mutationFn: dispatchContentReports,
    onSuccess: (res) => {
      setOpen(false);
      showSuccess(`Dispatched: ${res.dispatched}`);
      void qc.invalidateQueries({ queryKey: ["admin", "content-reports"] });
    },
    onError: showError,
  });

  const retryMut = useMutation({
    mutationFn: retryContentReport,
    onSuccess: () => void qc.invalidateQueries({ queryKey: ["admin", "content-reports"] }),
    onError: showError,
  });

  return (
    <Page
      title={t("content_safety.title")}
      actions={
        <Button theme="solid" onClick={() => setOpen(true)}>
          {t("content_safety.dispatch_all")}
        </Button>
      }
    >
      {isLoading ? (
        <Spin />
      ) : (
        <Card>
          <Table<ContentSafetyReport>
            dataSource={data?.items ?? []}
            rowKey="id"
            pagination={{ currentPage: page, pageSize: 50, total: data?.meta?.total ?? 0, onPageChange: setPage }}
            columns={[
              { title: "ID", dataIndex: "id", render: (v) => <Link to={`/content-safety/reports/${v}`}>#{v}</Link> },
              { title: t("content_safety.col_source"), dataIndex: "source" },
              {
                title: t("content_safety.col_excerpt"),
                dataIndex: "content_excerpt",
                render: (v: string) => <span style={{ maxWidth: 360, display: "inline-block" }}>{v}</span>,
              },
              { title: t("content_safety.col_status"), dataIndex: "status", render: (v: string) => <Tag>{v}</Tag> },
              { title: t("content_safety.col_remote_ref"), dataIndex: "remote_ref", render: (v) => v ?? "—" },
              { title: t("content_safety.col_retries"), dataIndex: "retries" },
              {
                title: "",
                render: (_, r: ContentSafetyReport) =>
                  r.status === "failed" && (
                    <Button size="small" onClick={() => retryMut.mutate(r.id)}>
                      {t("content_safety.retry")}
                    </Button>
                  ),
              },
            ]}
          />
        </Card>
      )}
      <Modal
        title={t("content_safety.dispatch_all")}
        visible={open}
        onCancel={() => setOpen(false)}
        onOk={() => dispatchMut.mutate(Number(batch))}
        confirmLoading={dispatchMut.isPending}
      >
        <Field label="Batch size">
          <Input value={batch} onChange={setBatch} aria-label="batch" />
        </Field>
      </Modal>
    </Page>
  );
}

export function ContentSafetyReportDetail(): JSX.Element {
  const { t } = useTranslation();
  const { id } = useParams<{ id: string }>();
  const cid = Number(id ?? 0);
  const qc = useQueryClient();
  const { showError } = useApiToast();
  const { data, isLoading } = useQuery({
    queryKey: ["admin", "content-report", cid],
    queryFn: () => getContentReport(cid),
    enabled: cid > 0,
  });
  const retryMut = useMutation({
    mutationFn: () => retryContentReport(cid),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ["admin", "content-report", cid] }),
    onError: showError,
  });

  if (isLoading || !data) return <Spin />;

  return (
    <Page
      title={`Report #${data.id}`}
      actions={
        data.status === "failed" && (
          <Button onClick={() => retryMut.mutate()} loading={retryMut.isPending}>
            {t("content_safety.retry")}
          </Button>
        )
      }
    >
      <Banner type="info" description="12377 上报内容仅授权人员可见" closeIcon={null} />
      <Card style={{ marginTop: 12 }}>
        <Descriptions
          data={[
            { key: "Source", value: data.source },
            { key: "Status", value: <Tag>{data.status}</Tag> },
            { key: "Remote ref", value: data.remote_ref ?? "—" },
            { key: "Retries", value: data.retries },
          ]}
        />
        <h3>Full content</h3>
        <pre style={{ background: "#f3f4f6", padding: 12, whiteSpace: "pre-wrap" }}>{data.full_content}</pre>
      </Card>
    </Page>
  );
}
