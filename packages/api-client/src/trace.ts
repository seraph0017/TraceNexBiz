// trace_id 透传 helper（frontend §15）.
export function propagateTrace(headers: Record<string, string>, traceId: string): Record<string, string> {
  return { ...headers, 'X-Oneapi-Request-Id': traceId };
}
