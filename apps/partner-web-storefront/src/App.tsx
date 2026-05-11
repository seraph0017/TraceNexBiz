// 路由树（per frontend §3.1）
//   /                     Home
//   /models               Models
//   /pricing              Pricing
//   /apply-partner        ApplyPartner（多步表单 + KYC + PIPL 单独同意）
//   /legal/:doc           Legal（privacy / terms / partner / algorithm_filing / gen_ai_filing /
//                                 platform_qualification / dpo / complaint / children）
//   *                     NotFound
import { Routes, Route, Navigate } from "react-router-dom";
import { Layout } from "@/components/Layout";
import { Home } from "@/pages/Home";
import { Models } from "@/pages/Models";
import { Pricing } from "@/pages/Pricing";
import { ApplyPartner } from "@/pages/ApplyPartner";
import { Legal } from "@/pages/Legal";
import { NotFound } from "@/pages/NotFound";

export function App(): JSX.Element {
  return (
    <Routes>
      <Route element={<Layout />}>
        <Route path="/" element={<Home />} />
        <Route path="/models" element={<Models />} />
        <Route path="/pricing" element={<Pricing />} />
        <Route path="/apply-partner" element={<ApplyPartner />} />
        {/* 旧路由名 partner-apply 重定向，保持外链友好 */}
        <Route path="/partner-apply" element={<Navigate to="/apply-partner" replace />} />
        <Route path="/legal/:doc" element={<Legal />} />
        <Route path="/legal" element={<Navigate to="/legal/privacy" replace />} />
        <Route path="*" element={<NotFound />} />
      </Route>
    </Routes>
  );
}
