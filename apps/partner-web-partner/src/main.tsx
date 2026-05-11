import React from "react";
import ReactDOM from "react-dom/client";
import { BrowserRouter } from "react-router-dom";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { App } from "./App";
import { ErrorBoundary } from "@/components/ErrorBoundary";
import { initI18n } from "@/i18n";

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 60_000,
      refetchOnWindowFocus: false,
      retry: 1,
    },
    mutations: { retry: 0 },
  },
});

async function bootstrap(): Promise<void> {
  await initI18n();
  const root = document.getElementById("root");
  if (!root) throw new Error("#root element not found");
  ReactDOM.createRoot(root).render(
    <React.StrictMode>
      <ErrorBoundary>
        <QueryClientProvider client={queryClient}>
          <BrowserRouter>
            <App />
          </BrowserRouter>
        </QueryClientProvider>
      </ErrorBoundary>
    </React.StrictMode>,
  );
}

void bootstrap();
