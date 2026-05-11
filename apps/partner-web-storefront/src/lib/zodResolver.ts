// 极简 zod resolver —— 替代 @hookform/resolvers/zod，避免新增依赖
// 与 react-hook-form Resolver 接口契合（v7）：返回 { values, errors }
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
  // 仅取顶层 path[0] 作为 field key —— 嵌套 path 暂不需要（form 都是平面）
  const key = (issue.path[0] ?? "_root") as string;
  if (target[key]) return; // first-error-wins
  target[key] = { type: issue.code, message: issue.message };
}
