import type { MouseEvent, ReactNode } from 'react'
import { type KycDocument, documentFileUrl, downloadDocumentFile } from '@/hooks/useKyc'
import { handleApiError } from '@/lib/api-errors'

interface DocumentFileLinkProps {
  doc: KycDocument
  className?: string
  children?: ReactNode
}

// Ссылка на файл документа компании. По клику качаем через api, а не <a download>: так 401
// ведёт на логин, а 404/500 видны тостом, а не сохраняются под видом файла. href оставлен —
// с модификатором (новая вкладка) ссылка работает как обычная.
export function DocumentFileLink({ doc, className, children = doc.file_name }: DocumentFileLinkProps) {
  const handleClick = async (e: MouseEvent<HTMLAnchorElement>) => {
    if (e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return
    e.preventDefault()
    try {
      const url = URL.createObjectURL(await downloadDocumentFile(doc.id))
      const link = document.createElement('a')
      link.href = url
      link.download = doc.file_name
      link.click()
      setTimeout(() => URL.revokeObjectURL(url))
    } catch (err) {
      handleApiError(err)
    }
  }

  return (
    <a href={documentFileUrl(doc.id)} onClick={handleClick} className={className}>
      {children}
    </a>
  )
}
