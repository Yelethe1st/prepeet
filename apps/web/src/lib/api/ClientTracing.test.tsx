import { render } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { ClientTracing } from "./ClientTracing";
import { resetTracingForTests, traceHeaders } from "./tracing";

const pathname = vi.hoisted(() => ({ current: "/practice" }));
vi.mock("next/navigation", () => ({
  usePathname: () => pathname.current,
}));

/**
 * A mechanism nobody calls is the same broken link with more code behind it,
 * so these assert the wiring rather than the generator.
 */
describe("ClientTracing", () => {
  afterEach(() => {
    resetTracingForTests();
    vi.unstubAllEnvs();
  });

  it("starts a trace on mount when tracing is enabled", () => {
    vi.stubEnv("NEXT_PUBLIC_TRACING", "true");
    resetTracingForTests({ enabled: true });

    render(<ClientTracing />);

    expect(traceHeaders().traceparent).toBeTruthy();
  });

  it("starts nothing when tracing is not enabled", () => {
    vi.stubEnv("NEXT_PUBLIC_TRACING", "false");
    resetTracingForTests({ enabled: false });

    render(<ClientTracing />);

    expect(traceHeaders()).toEqual({});
  });

  it("starts a new trace when the screen changes", () => {
    vi.stubEnv("NEXT_PUBLIC_TRACING", "true");
    resetTracingForTests({ enabled: true });
    const { rerender } = render(<ClientTracing />);
    const before = traceHeaders().traceparent?.split("-")[1];

    pathname.current = "/complete";
    rerender(<ClientTracing />);
    const after = traceHeaders().traceparent?.split("-")[1];

    // One navigation is one trace. Carrying the previous screen's trace into
    // the next would merge two waits nobody experienced as one.
    expect(after).not.toBe(before);
    pathname.current = "/practice";
  });

  it("renders nothing", () => {
    // It exists for its effects. Rendering anything would make a layout
    // concern into a visual one.
    const { container } = render(<ClientTracing />);

    expect(container.innerHTML).toBe("");
  });
});
