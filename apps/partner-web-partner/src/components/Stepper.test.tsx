import { describe, it, expect } from "vitest";
import { render } from "@testing-library/react";
import { Stepper } from "./Stepper";

describe("Stepper", () => {
  it("renders steps with status badges", () => {
    const { getByText, container } = render(
      <Stepper
        steps={[
          { label: "锁定", status: "done" },
          { label: "增加额度", status: "active" },
          { label: "对账", status: "pending" },
        ]}
      />,
    );
    expect(getByText("锁定")).toBeInTheDocument();
    expect(container.querySelector("[aria-current='step']")).toBeTruthy();
  });
});
