// 顶层错误边界 —— 捕获 React 渲染异常 / async hook 报错（部分）
// 配合 toast 与 retry 按钮；不依赖 Sentry（前端 §16 占位）
import * as React from "react";

interface ErrorBoundaryProps {
  children: React.ReactNode;
  fallback?: (error: Error, reset: () => void) => React.ReactNode;
}

interface ErrorBoundaryState {
  error: Error | null;
}

export class ErrorBoundary extends React.Component<ErrorBoundaryProps, ErrorBoundaryState> {
  override state: ErrorBoundaryState = { error: null };

  static getDerivedStateFromError(error: Error): ErrorBoundaryState {
    return { error };
  }

  override componentDidCatch(error: Error, info: React.ErrorInfo): void {
    // 不调用 console.log（hooks 规则禁用）；接 logger 后替换
    if (typeof window !== "undefined") {
      const w = window as unknown as { __TNBIZ_ERR__?: Array<{ error: Error; info: React.ErrorInfo }> };
      w.__TNBIZ_ERR__ = (w.__TNBIZ_ERR__ ?? []).concat([{ error, info }]);
    }
  }

  reset = (): void => this.setState({ error: null });

  override render(): React.ReactNode {
    if (this.state.error) {
      if (this.props.fallback) return this.props.fallback(this.state.error, this.reset);
      return (
        <div role="alert" style={{ padding: 24 }}>
          <h2>页面加载异常</h2>
          <p style={{ color: "#888" }}>{this.state.error.message}</p>
          <button type="button" onClick={this.reset}>
            重试
          </button>
        </div>
      );
    }
    return this.props.children;
  }
}
