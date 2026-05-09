import type {
  ComponentBaseContext,
  ComponentBaseMapper,
  ExecutionDetailsContext,
  ExecutionInfo,
  NodeInfo,
  OutputPayload,
  SubtitleContext,
} from "../types";
import type { ComponentBaseProps, EventSection } from "@/ui/componentBase";
import type React from "react";
import type { MetadataItem } from "@/ui/metadataList";
import { noopMapper } from "../noop";
import { renderTimeAgo } from "@/components/TimeAgo";
import { getBackgroundColorClass, getColorClass } from "@/lib/colors";
import { getTriggerRenderer, getState, getStateMap } from "..";
import { stringOrDash } from "../utils";
import HetznerRobotIcon from "@/assets/icons/integrations/hetzner_robot.svg";

type HetznerRobotConfiguration = {
  server?: unknown;
  resetType?: unknown;
  os?: unknown;
  name?: unknown;
  dist?: unknown;
  fingerprint?: unknown;
  ipVersion?: unknown;
  protocol?: unknown;
  action?: unknown;
  authorizedKeys?: unknown;
};

function getConfigString(value: unknown): string | undefined {
  if (typeof value === "string") {
    const trimmed = value.trim();
    return trimmed.length > 0 ? trimmed : undefined;
  }
  if (typeof value === "number") {
    return String(value);
  }
  if (!value || typeof value !== "object") {
    return undefined;
  }
  const obj = value as Record<string, unknown>;
  for (const field of ["label", "displayName", "name", "value", "id"]) {
    const candidate = obj[field];
    if (typeof candidate === "string" && candidate.trim().length > 0) {
      return candidate.trim();
    }
  }
  return undefined;
}

function metadataList(node: NodeInfo): MetadataItem[] {
  const metadata: MetadataItem[] = [];
  const configuration = (node.configuration as HetznerRobotConfiguration | undefined) ?? {};

  const server = getConfigString(configuration.server);
  const resetType = getConfigString(configuration.resetType);
  const os = getConfigString(configuration.os);
  const name = getConfigString(configuration.name);
  const dist = getConfigString(configuration.dist);
  const fingerprint = getConfigString(configuration.fingerprint);
  const action = getConfigString(configuration.action);

  if (server) metadata.push({ icon: "server", label: `Server: ${server}` });
  if (resetType) metadata.push({ icon: "refresh-cw", label: `Reset: ${resetType}` });
  if (os) metadata.push({ icon: "terminal", label: `OS: ${os}` });
  if (name) metadata.push({ icon: "tag", label: `Name: ${name}` });
  if (dist) metadata.push({ icon: "hard-drive", label: `Distro: ${dist}` });
  if (fingerprint) metadata.push({ icon: "key", label: `Key: ${fingerprint}` });
  if (action) metadata.push({ icon: "shield", label: `Action: ${action}` });

  return metadata;
}

function getExecutionDetails(context: ExecutionDetailsContext): Record<string, string> {
  const details: Record<string, string> = {};
  const outputs = context.execution.outputs as Record<string, OutputPayload[] | undefined> | undefined;
  const payload = outputs?.default?.[0]?.data;
  const payloadRecord = payload && typeof payload === "object" ? (payload as Record<string, unknown>) : undefined;
  const nested = payloadRecord?.data;
  const source = nested && typeof nested === "object" ? (nested as Record<string, unknown>) : payloadRecord;

  if (source) {
    if (source.serverNumber !== undefined && source.serverNumber !== null)
      details["Server"] = stringOrDash(source.serverNumber);
    if (source.serverName !== undefined && source.serverName !== null)
      details["Server Name"] = stringOrDash(source.serverName);
    if (source.status !== undefined && source.status !== null) details["Status"] = stringOrDash(source.status);
    if (source.resetType !== undefined && source.resetType !== null)
      details["Reset Type"] = stringOrDash(source.resetType);
    if (source.os !== undefined && source.os !== null) details["OS"] = stringOrDash(source.os);
    if (source.name !== undefined && source.name !== null) details["Name"] = stringOrDash(source.name);
    if (source.fingerprint !== undefined && source.fingerprint !== null)
      details["Fingerprint"] = stringOrDash(source.fingerprint);
    if (source.dist !== undefined && source.dist !== null) details["Distribution"] = stringOrDash(source.dist);
    if (source.ruleCount !== undefined && source.ruleCount !== null) details["Rules"] = stringOrDash(source.ruleCount);
    if (source.serverCount !== undefined && source.serverCount !== null)
      details["Servers"] = stringOrDash(source.serverCount);
  }

  if (context.execution.createdAt) {
    details["Started at"] = new Date(context.execution.createdAt).toLocaleString();
  }
  if (context.execution.updatedAt && context.execution.state === "STATE_FINISHED") {
    details["Finished at"] = new Date(context.execution.updatedAt).toLocaleString();
  }

  return details;
}

function props(context: ComponentBaseContext): ComponentBaseProps {
  const lastExecution = context.lastExecutions.length > 0 ? context.lastExecutions[0] : undefined;
  const componentName = context.componentDefinition.name ?? "hetznerRobot";

  return {
    title:
      context.node.name || context.componentDefinition.label || context.componentDefinition.name || "Unnamed component",
    iconSrc: HetznerRobotIcon,
    iconSlug: context.componentDefinition.icon || "server",
    iconColor: getColorClass(context.componentDefinition?.color || "gray"),
    collapsed: context.node.isCollapsed,
    collapsedBackground: getBackgroundColorClass(context.componentDefinition?.color || "white"),
    eventSections: lastExecution ? getEventSections(context.nodes, lastExecution, componentName) : undefined,
    includeEmptyState: !lastExecution,
    metadata: metadataList(context.node),
    eventStateMap: getStateMap(componentName),
  };
}

function subtitle(context: SubtitleContext): string | React.ReactNode {
  const timestamp = context.execution.updatedAt || context.execution.createdAt;
  return timestamp ? renderTimeAgo(new Date(timestamp)) : "";
}

function getEventSections(nodes: NodeInfo[], execution: ExecutionInfo, componentName: string): EventSection[] {
  const rootTriggerNode = nodes.find((n) => n.id === execution.rootEvent?.nodeId);
  const rootTriggerRenderer = getTriggerRenderer(rootTriggerNode?.componentName || "");
  const { title } = rootTriggerRenderer.getTitleAndSubtitle({ event: execution.rootEvent });
  const subtitleTimestamp = execution.updatedAt || execution.createdAt;
  const eventSubtitle = subtitleTimestamp ? renderTimeAgo(new Date(subtitleTimestamp)) : "";

  return [
    {
      receivedAt: execution.createdAt ? new Date(execution.createdAt) : new Date(),
      eventTitle: title,
      eventSubtitle,
      eventState: getState(componentName)(execution),
      eventId: execution.rootEvent?.id || execution.id || "execution",
    },
  ];
}

export const hetznerRobotBaseMapper: ComponentBaseMapper = {
  ...noopMapper,
  props,
  getExecutionDetails,
  subtitle,
};
