// 顶层 ErrorBoundary —— 子树渲染错误兜底
import * as React from "react";

interface State {
  err: Error | null;
}

export class ErrorBoundary extends React.Component<
  { children: React.ReactNode },
  State
> {
  override state: State = { err: null };

  static getDerivedStateFromError(err: Error): State {
    return { err };
  }

  override componentDidCatch(_err: Error, _info: React.ErrorInfo): void {
    // 服务端日志已由后端 trace_id；这里不重复 console
  }

  override render(): React.ReactNode {
    if (this.state.err) {
      return (
        <div role="alert" style={{ padding: 24, color: "#dc2626" }}>
          <h2>页面渲染异常</h2>
          <pre style={{ whiteSpace: "pre-wrap" }}>{this.state.err.message}</pre>
        </div>
      );
    }
    return this.props.children;
  }
}
