// DEPRECATED — types now live in ./envelope.ts. Kept as a re-export for
// backward compatibility with code that imports `ApiEnvelope` / `ApiError`
// from `@tnbiz/api-client/types`. New code should import from the package
// root (which re-exports everything from envelope.ts).
export type { ApiEnvelope, ApiError, PageMeta, PaginatedEnvelope } from "./envelope";
export { ApiException } from "./envelope";
