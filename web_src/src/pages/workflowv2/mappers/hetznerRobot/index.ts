import type { ComponentBaseMapper, EventStateRegistry, TriggerRenderer } from "../types";
import { buildActionStateRegistry } from "../utils";
import { hetznerRobotBaseMapper } from "./base";

export const componentMappers: Record<string, ComponentBaseMapper> = {
  getServer: hetznerRobotBaseMapper,
  resetServer: hetznerRobotBaseMapper,
  enableRescue: hetznerRobotBaseMapper,
  disableRescue: hetznerRobotBaseMapper,
  listSshKeys: hetznerRobotBaseMapper,
  addSshKey: hetznerRobotBaseMapper,
  deleteSshKey: hetznerRobotBaseMapper,
  installLinux: hetznerRobotBaseMapper,
  cancelLinuxInstall: hetznerRobotBaseMapper,
  listFirewallRules: hetznerRobotBaseMapper,
  addFirewallRule: hetznerRobotBaseMapper,
  updateFirewallRule: hetznerRobotBaseMapper,
  deleteFirewallRule: hetznerRobotBaseMapper,
  wakeOnLan: hetznerRobotBaseMapper,
  renameServer: hetznerRobotBaseMapper,
  listServers: hetznerRobotBaseMapper,
};

export const triggerRenderers: Record<string, TriggerRenderer> = {};

export const eventStateRegistry: Record<string, EventStateRegistry> = {
  getServer: buildActionStateRegistry("fetched"),
  resetServer: buildActionStateRegistry("reset"),
  enableRescue: buildActionStateRegistry("enabled"),
  disableRescue: buildActionStateRegistry("disabled"),
  listSshKeys: buildActionStateRegistry("listed"),
  addSshKey: buildActionStateRegistry("added"),
  deleteSshKey: buildActionStateRegistry("deleted"),
  installLinux: buildActionStateRegistry("installed"),
  cancelLinuxInstall: buildActionStateRegistry("cancelled"),
  listFirewallRules: buildActionStateRegistry("listed"),
  addFirewallRule: buildActionStateRegistry("added"),
  updateFirewallRule: buildActionStateRegistry("updated"),
  deleteFirewallRule: buildActionStateRegistry("deleted"),
  wakeOnLan: buildActionStateRegistry("woken"),
  renameServer: buildActionStateRegistry("renamed"),
  listServers: buildActionStateRegistry("listed"),
};
