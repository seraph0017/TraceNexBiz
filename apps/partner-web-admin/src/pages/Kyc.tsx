// KYC 列表 + 详情审核 + 三方核验
import { useState } from "react";
import { Link, useParams } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Banner, Button, Card, Descriptions, Input, Modal, Spin, Table, Tag, TextArea } from "@douyinfe/semi-ui";
import { useTranslation } from "react-i18next";
import { Page } from "@/components/Page";
import { Field } from "@/components/Field";
import { listKyc, getKyc, reviewKyc, callThirdPartyCheck, type KycSubmission } from "@/api/admin";
import { useApiToast } from "@/hooks/useApiToast";

export function KycList(): JSX.Element {
  const { t } = useTranslation();
  const [page, setPage] = useState(1);
  const { data, isLoading } = useQuery({
    queryKey: ["admin", "kyc", page],
    queryFn: () => listKyc({ page, limit: 50 }),
    placeholderData: (prev) => prev,
  });
  return (
    <Page title={t("kyc.title")}>
      {isLoading ? (
        <Spin />
      ) : (
        <Card>
          <Table<KycSubmission>
            dataSource={data?.items ?? []}
            rowKey="id"
            pagination={{ currentPage: page, pageSize: 50, total: data?.meta?.total ?? 0, onPageChange: setPage }}
            columns={[
              { title: t("kyc.col_id"), dataIndex: "id", render: (v) => <Link to={`/kyc/${v}`}>#{v}</Link> },
              { title: t("kyc.col_subject_kind"), dataIndex: "subject_kind" },
              { title: t("kyc.col_subject_name"), dataIndex: "subject_name" },
              { title: t("kyc.col_status"), dataIndex: "status", render: (v: string) => <Tag>{v}</Tag> },
              { title: "Created", dataIndex: "created_at" },
            ]}
          />
        </Card>
      )}
    </Page>
  );
}

export function KycDetail(): JSX.Element {
  const { t } = useTranslation();
  const { id } = useParams<{ id: string }>();
  const kid = Number(id ?? 0);
  const qc = useQueryClient();
  const { showError, showSuccess } = useApiToast();
  const [rejectOpen, setRejectOpen] = useState(false);
  const [rejectCode, setRejectCode] = useState("");
  const [rejectText, setRejectText] = useState("");

  const { data, isLoading } = useQuery({
    queryKey: ["admin", "kyc", kid],
    queryFn: () => getKyc(kid),
    enabled: kid > 0,
  });

  const reviewMut = useMutation({
    mutationFn: (input: { approve: boolean; reject_code?: string; reject_text?: string }) => reviewKyc(kid, input),
    onSuccess: () => {
      showSuccess(t("app.confirm"));
      setRejectOpen(false);
      void qc.invalidateQueries({ queryKey: ["admin", "kyc", kid] });
    },
    onError: showError,
  });

  const checkMut = useMutation({
    mutationFn: () => callThirdPartyCheck(kid),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ["admin", "kyc", kid] }),
    onError: showError,
  });

  if (isLoading || !data) return <Spin />;

  return (
    <Page
      title={`KYC #${data.id}`}
      actions={
        <>
          <Button onClick={() => checkMut.mutate()} loading={checkMut.isPending}>
            {t("kyc.third_party")}
          </Button>
          <Button theme="solid" type="primary" loading={reviewMut.isPending} onClick={() => reviewMut.mutate({ approve: true })}>
            {t("kyc.approve")}
          </Button>
          <Button type="danger" onClick={() => setRejectOpen(true)}>
            {t("kyc.reject")}
          </Button>
        </>
      }
    >
      <Card>
        <Descriptions
          data={[
            { key: t("kyc.col_subject_kind"), value: data.subject_kind },
            { key: "Subject", value: `${data.subject_name} (#${data.subject_id})` },
            { key: t("kyc.col_status"), value: <Tag>{data.status}</Tag> },
            { key: "Real name", value: data.real_name_masked },
            { key: "ID card", value: data.id_card_masked },
            { key: t("kyc.third_party_result"), value: data.third_party_check ? `${data.third_party_check.provider} / ${data.third_party_check.status}` : "—" },
          ]}
        />
        {data.documents.length > 0 && (
          <div style={{ marginTop: 12 }}>
            {data.documents.map((d) => (
              <a key={d.url} href={d.url} target="_blank" rel="noopener noreferrer" style={{ marginRight: 12 }}>
                {d.kind}
              </a>
            ))}
          </div>
        )}
        {data.status === "rejected" && (
          <Banner type="warning" description={`Already rejected`} closeIcon={null} style={{ marginTop: 12 }} />
        )}
      </Card>
      <Modal
        title={t("kyc.reject")}
        visible={rejectOpen}
        onCancel={() => setRejectOpen(false)}
        onOk={() => reviewMut.mutate({ approve: false, reject_code: rejectCode, reject_text: rejectText })}
        confirmLoading={reviewMut.isPending}
        okType="danger"
      >
        <Field label={t("kyc.reject_code")}>
          <Input value={rejectCode} onChange={setRejectCode} aria-label="reject-code" />
        </Field>
        <Field label={t("kyc.reject_text")}>
          <TextArea value={rejectText} onChange={setRejectText} rows={3} aria-label="reject-text" />
        </Field>
      </Modal>
    </Page>
  );
}
