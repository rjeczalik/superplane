import type { ComponentBaseMapper, ExecutionDetailsContext, TriggerRenderer } from "../../types";
import { noopMapper } from "../../noop";
import { defaultTriggerRenderer } from "../../default";

function terraformDetails(context: ExecutionDetailsContext): Record<string, string> {
  const outputs = context.execution.outputs as { default?: Array<{ data?: Record<string, unknown> }> } | undefined;
  const data = outputs?.default?.[0]?.data || {};
  const details: Record<string, string> = {};

  for (const key of ["managed_resource_id", "resource_type", "remote_id", "display_name", "status", "health"]) {
    const value = data[key];
    if (value !== undefined && value !== null) {
      details[key.replace(/_/g, " ")] = String(value);
    }
  }

  return details;
}

function mapper(): ComponentBaseMapper {
  return {
    ...noopMapper,
    getExecutionDetails: terraformDetails,
  };
}

export const terraformCreateMapper = mapper();
export const terraformReadMapper = mapper();
export const terraformUpdateMapper = mapper();
export const terraformDeleteMapper = mapper();
export const terraformOnChangedTriggerRenderer: TriggerRenderer = defaultTriggerRenderer;

export function mapperForTerraformComponent(name: string): ComponentBaseMapper | undefined {
  if (name.endsWith(".create")) return terraformCreateMapper;
  if (name.endsWith(".read")) return terraformReadMapper;
  if (name.endsWith(".update")) return terraformUpdateMapper;
  if (name.endsWith(".delete")) return terraformDeleteMapper;
  return undefined;
}

export function rendererForTerraformTrigger(name: string): TriggerRenderer | undefined {
  return name.endsWith(".onChanged") ? terraformOnChangedTriggerRenderer : undefined;
}
