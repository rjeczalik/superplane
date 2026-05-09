You are now connected to the Hetzner Robot API.

{{- if eq .ServerCount 0 }}
We did not find any dedicated servers on this account yet. You can still configure capabilities — they will run against any servers added later.
{{- else if eq .ServerCount 1 }}
We found **1 dedicated server** on this account.
{{- else }}
We found **{{ .ServerCount }} dedicated servers** on this account.
{{- end }}

You can now use the selected Hetzner Robot capabilities in your stages.
