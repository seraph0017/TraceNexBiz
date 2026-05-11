// 404
import { Link } from "react-router-dom";
import { Empty } from "@douyinfe/semi-ui";

export function NotFound(): JSX.Element {
  return (
    <main style={{ padding: 64, textAlign: "center" }}>
      <Empty title="404" description={<Link to="/dashboard">返回控制台</Link>} />
    </main>
  );
}
