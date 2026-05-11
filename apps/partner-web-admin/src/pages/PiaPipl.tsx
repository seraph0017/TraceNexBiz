// PIA + PIPL complaints
import { useState } from "react";
import { Link, useParams } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Button, Card, Descriptions, Input, Modal, Spin, Table, Tag, TextArea } from "@douyinfe/semi-ui";
import { useTranslation } from "react-i18next";
import { Page } from "@/components/Page";
import { Field } from "@/components/Field";
import {
  listPia,
  generatePia,
  listPiplComplaints,
  getPiplComplaint,
  resolvePiplComplaint,
  type PiaReport,
  type PiplComplaint,
} from "@/api/admin";
import { useApiToast } from "@/hooks/useApiToast";

export function Pia(): JSX.Element {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const { showError, showSuccess } = useApiToast();
  const [open, setOpen] = useState(false);
  const [scope, setScope] = useState("");

  const { data, isLoading } = useQuery({
    queryKey: ["admin", "pia"],
    queryFn: listPia,
  });
  const mut = useMutation({
    mutationFn: generatePia,
    onSuccess: () => {
      setOpen(false);
      showSuccess(t("pia.generate"));
      void qc.invalidateQueries({ queryKey: ["admin", "pia"] });
    },
    onError: showError,
  });

  return (
    <Page title={t("pia.title")} actions={<Button theme="solid" onClick={() => setOpen(true)}>{t("pia.generate")}</Button>}>
      {isLoading ? (
        <Spin />
      ) : (
        <Card>
          <Table<PiaReport>
            dataSource={data ?? []}
            rowKey="id"
            pagination={false}
            columns={[
              { title: "ID", dataIndex: "id" },
              { title: t("pia.col_scope"), dataIndex: "scope" },
              { title: t("pia.col_status"), dataIndex: "status", render: (v: string) => <Tag>{v}</Tag> },
              { title: t("pia.col_generated"), dataIndex: "generated_at" },
              {
                title: "",
                render: (_, r: PiaReport) =>
                  r.download_url && (
                    <a href={r.download_url} download>
                      {t("pia.download")}
                    </a>
                  ),
              },
            ]}
          />
        </Card>
      )}
      <Modal title={t("pia.generate")} visible={open} onCancel={() => setOpen(false)} onOk={() => mut.mutate(scope)} confirmLoading={mut.isPending}>
        <Field label={t("pia.scope")}>
          <Input value={scope} onChange={setScope} aria-label="scope" />
        </Field>
      </Modal>
    </Page>
  );
}

export function PiplComplaints(): JSX.Element {
  const { t } = useTranslation();
  const { data, isLoading } = useQuery({
    queryKey: ["admin", "pipl-complaints"],
    queryFn: listPiplComplaints,
  });
  return (
    <Page title={t("pipl_complaints.title")}>
      {isLoading ? (
        <Spin />
      ) : (
        <Card>
          <Table<PiplComplaint>
            dataSource={data ?? []}
            rowKey="id"
            pagination={false}
            columns={[
              { title: "ID", dataIndex: "id", render: (v) => <Link to={`/pipl-complaints/${v}`}>#{v}</Link> },
              { title: t("pipl_complaints.col_kind"), dataIndex: "kind" },
              { title: t("pipl_complaints.col_customer"), dataIndex: "customer_name" },
              { title: t("pipl_complaints.col_status"), dataIndex: "status", render: (v: string) => <Tag>{v}</Tag> },
              { title: "Created", dataIndex: "created_at" },
            ]}
          />
        </Card>
      )}
    </Page>
  );
}

export function PiplComplaintDetail(): JSX.Element {
  const { t } = useTranslation();
  const { id } = useParams<{ id: string }>();
  const cid = Number(id ?? 0);
  const qc = useQueryClient();
  const { showError, showSuccess } = useApiToast();
  const [decision, setDecision] = useState<"resolved" | "rejected">("resolved");
  const [note, setNote] = useState("");

  const { data, isLoading } = useQuery({
    queryKey: ["admin", "pipl-complaint", cid],
    queryFn: () => getPiplComplaint(cid),
    enabled: cid > 0,
  });

  const mut = useMutation({
    mutationFn: () => resolvePiplComplaint(cid, { decision, note }),
    onSuccess: () => {
      showSuccess(t("pipl_complaints.resolve"));
      void qc.invalidateQueries({ queryKey: ["admin", "pipl-complaint", cid] });
    },
    onError: showError,
  });

  if (isLoading || !data) return <Spin />;

  return (
    <Page title={`Complaint #${data.id}`}>
      <Card>
        <Descriptions
          data={[
            { key: "Kind", value: data.kind },
            { key: "Customer", value: `${data.customer_name} (#${data.customer_id})` },
            { key: "Status", value: <Tag>{data.status}</Tag> },
            { key: "Detail", value: data.detail },
          ]}
        />
        <h3>Timeline</h3>
        <ul>
          {data.timeline.map((e, i) => (
            <li key={i}>
              <strong>{e.ts}</strong>: {e.note}
            </li>
          ))}
        </ul>
      </Card>
      {data.status !== "resolved" && data.status !== "rejected" && (
        <Card title={t("pipl_complaints.resolve")} style={{ marginTop: 12 }}>
          <Field label={t("pipl_complaints.decision")}>
            <select value={decision} onChange={(e) => setDecision(e.target.value as "resolved" | "rejected")}>
              <option value="resolved">{t("pipl_complaints.decision_resolved")}</option>
              <option value="rejected">{t("pipl_complaints.decision_rejected")}</option>
            </select>
          </Field>
          <Field label={t("pipl_complaints.note")}>
            <TextArea value={note} onChange={setNote} rows={3} aria-label="note" />
          </Field>
          <Button theme="solid" type="primary" loading={mut.isPending} onClick={() => mut.mutate()}>
            {t("app.submit")}
          </Button>
        </Card>
      )}
    </Page>
  );
}
