// 404
import { Link } from "react-router-dom";
import { Page } from "@/components/Page";

export function NotFound(): JSX.Element {
  return (
    <Page title="404">
      <p>Not found</p>
      <Link to="/partners">{"<- partners"}</Link>
    </Page>
  );
}
