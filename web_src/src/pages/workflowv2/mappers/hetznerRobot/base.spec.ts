import { describe, expect, it } from "vitest";

import { hetznerRobotBaseMapper } from "./base";
import type { ExecutionDetailsContext, ExecutionInfo, NodeInfo, OutputPayload } from "../types";

function buildNode(overrides?: Partial<NodeInfo>): NodeInfo {
  return {
    id: "node-1",
    name: "List Servers",
    componentName: "hetznerRobot.listServers",
    isCollapsed: false,
    configuration: {},
    metadata: {},
    ...overrides,
  };
}

function buildExecution(overrides?: Partial<ExecutionInfo>): ExecutionInfo {
  return {
    id: "exec-1",
    createdAt: "2026-05-05T10:00:00Z",
    updatedAt: "2026-05-05T10:01:00Z",
    state: "STATE_FINISHED",
    result: "RESULT_PASSED",
    resultReason: "RESULT_REASON_OK",
    resultMessage: "",
    metadata: {},
    configuration: {},
    rootEvent: undefined,
    ...overrides,
  };
}

function buildOutput(data: unknown): OutputPayload {
  return {
    type: "hetznerRobot.server.listed",
    timestamp: "2026-05-05T10:01:00Z",
    data,
  };
}

function buildDetailsContext(overrides?: {
  node?: Partial<NodeInfo>;
  execution?: Partial<ExecutionInfo>;
}): ExecutionDetailsContext {
  const node = buildNode(overrides?.node);
  return {
    nodes: [node],
    node,
    execution: buildExecution(overrides?.execution),
  };
}

describe("hetznerRobotBaseMapper.getExecutionDetails", () => {
  it("reads fields from the emitted output payload", () => {
    const details = hetznerRobotBaseMapper.getExecutionDetails(
      buildDetailsContext({
        execution: {
          outputs: {
            default: [
              buildOutput({
                serverNumber: "12345",
                serverName: "web-01",
                status: "ready",
                serverCount: 2,
              }),
            ],
          },
        },
      }),
    );

    expect(details["Server"]).toBe("12345");
    expect(details["Server Name"]).toBe("web-01");
    expect(details["Status"]).toBe("ready");
    expect(details["Servers"]).toBe("2");
  });

  it("shows zero server counts", () => {
    const details = hetznerRobotBaseMapper.getExecutionDetails(
      buildDetailsContext({
        execution: {
          outputs: {
            default: [
              buildOutput({
                serverCount: 0,
              }),
            ],
          },
        },
      }),
    );

    expect(details["Servers"]).toBe("0");
  });
});
