import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { Stepper } from "@/components/apply/Stepper";

describe("Stepper", () => {
  const steps = [
    { key: "a", label: "Step A" },
    { key: "b", label: "Step B" },
    { key: "c", label: "Step C" },
  ];

  it("当前步骤标记 aria-current=step", () => {
    render(<Stepper steps={steps} current="b" />);
    const items = screen.getAllByRole("listitem");
    expect(items[0]?.getAttribute("aria-current")).toBeNull();
    expect(items[1]?.getAttribute("aria-current")).toBe("step");
    expect(items[2]?.getAttribute("aria-current")).toBeNull();
  });

  it("已完成步骤显示 ✓", () => {
    render(<Stepper steps={steps} current="c" />);
    const text = screen.getByRole("list").textContent ?? "";
    expect(text).toContain("✓");
  });
});
