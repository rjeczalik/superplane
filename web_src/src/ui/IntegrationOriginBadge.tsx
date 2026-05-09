import { Badge } from "@/components/ui/badge";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/ui/tooltip";
import { Box } from "lucide-react";

type IntegrationOriginBadgeProps = {
  origin?: string;
  source?: string;
  version?: string;
  size?: "sm" | "md";
};

export function IntegrationOriginBadge({ origin, source, version, size = "sm" }: IntegrationOriginBadgeProps) {
  if (!origin || origin === "native") {
    return null;
  }

  if (origin !== "terraform") {
    return null;
  }

  const textSize = size === "md" ? "text-sm" : "text-xs";
  const tooltip = [source, version].filter(Boolean).join(" @ ");

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Badge variant="outline" className={textSize}>
          <Box aria-hidden="true" />
          Terraform
        </Badge>
      </TooltipTrigger>
      {tooltip && <TooltipContent>{tooltip}</TooltipContent>}
    </Tooltip>
  );
}
