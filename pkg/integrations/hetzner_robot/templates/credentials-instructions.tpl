Enter your Hetzner Robot **webservice** credentials. These are different from your main Hetzner account login — you create them separately in the Robot panel.

## How to create webservice credentials

1. Sign in to the [Hetzner Robot panel](https://robot.hetzner.com)
2. Open **Settings** → **Webservice and app settings**
3. Set (or reset) a username and password for the webservice user
4. Save the credentials and paste them below

## Required permission

{{- if eq .Permission "readWrite" }}
The capabilities you selected include actions that **modify** servers (renames, resets, SSH keys, rescue/Linux installs, firewall rules). The webservice user must therefore have **Read & Write** access.
{{- else }}
You only selected read-only capabilities, so a webservice user with **Read-only** access is sufficient. You can upgrade to Read & Write later if you enable mutating capabilities.
{{- end }}

> The credentials are stored encrypted and used only by SuperPlane to call the Hetzner Robot API on your behalf.
