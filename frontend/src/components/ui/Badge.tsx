import { cn } from '@/lib/utils'
import { type HTMLAttributes } from 'react'

interface BadgeProps extends HTMLAttributes<HTMLSpanElement> {
  variant?: 'default' | 'success' | 'warning' | 'danger' | 'info' | 'draft' | 'published' | 'agreed' | 'rejected'
}

export function Badge({ className, variant = 'default', children, ...props }: BadgeProps) {
  return (
    <span
      className={cn(
        'inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium transition-colors',
        {
          'bg-gray-100 text-gray-800': variant === 'default',
          'bg-success/10 text-success': variant === 'success',
          'bg-warning/10 text-warning': variant === 'warning',
          'bg-danger/10 text-danger': variant === 'danger',
          'bg-info/10 text-info': variant === 'info',
          'bg-gray-100 text-gray-700': variant === 'draft',
          'bg-blue-100 text-blue-700': variant === 'published',
          'bg-green-100 text-green-700': variant === 'agreed',
          'bg-red-100 text-red-700': variant === 'rejected',
        },
        className
      )}
      {...props}
    >
      {children}
    </span>
  )
}