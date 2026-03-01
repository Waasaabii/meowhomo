import React from "react"
import { cn } from "../lib/utils"

export interface IconProps extends React.SVGProps<SVGSVGElement> {
    name: string
    prefix?: string
}

export function Icon({ name, prefix = "icon", className, style, ...props }: IconProps) {
    const symbolId = `#${prefix}-${name}`

    return (
        <svg
            {...props}
            aria-hidden="true"
            className={cn("inline-block w-[1em] h-[1em] fill-current", className)}
            style={style}
        >
            <use href={symbolId} />
        </svg>
    )
}
