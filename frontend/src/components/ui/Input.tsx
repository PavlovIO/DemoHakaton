import { forwardRef, type InputHTMLAttributes } from 'react'
import { cn } from '@/lib/utils'

export const Input = forwardRef<HTMLInputElement, InputHTMLAttributes<HTMLInputElement>>(
  ({ className, ...props }, ref) => (
    <input
      ref={ref}
      className={cn(
        'w-full rounded border border-gray-300 px-3 py-2 text-sm outline-none',
        'focus:border-blue-500 focus:ring-2 focus:ring-blue-200',
        'disabled:bg-gray-100 disabled:text-gray-500',
        className,
      )}
      {...props}
    />
  ),
)
Input.displayName = 'Input'