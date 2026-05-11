// 极简 zod resolver —— 与 storefront 一致；平面 form 路径
import type { Resolver, FieldValues, FieldErrors } from "react-hook-form";
import type { ZodTypeAny, ZodIssue } from "zod";

export function zodResolver<TSchema extends ZodTypeAny, TFieldValues extends FieldValues = FieldValues>(
  schema: TSchema,
): Resolver<TFieldValues> {
  return async (values) => {
    const parsed = schema.safeParse(values);
    if (parsed.success) {
      return { values: parsed.data as TFieldValues, errors: {} };
    }
    const errors: FieldErrors<TFieldValues> = {};
    for (const issue of parsed.error.issues) {
      assignIssue(errors as Record<string, unknown>, issue);
    }
    return { values: {} as TFieldValues, errors };
  };
}

function assignIssue(target: Record<string, unknown>, issue: ZodIssue): void {
  const key = (issue.path[0] ?? "_root") as string;
  if (target[key]) return;
  target[key] = { type: issue.code, message: issue.message };
}
