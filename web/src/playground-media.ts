import type { PlaygroundMedia } from './types'

// Only media URLs are rendered; model text is never interpreted as HTML.
export function imageMedia(value: string, mime = ''): PlaygroundMedia | undefined {
  if (!value) return undefined
  if (/^data:image\/[a-z0-9.+-]+;base64,/i.test(value)) {
    return { mime: /^data:([^;,]+)/i.exec(value)![1]!, url: value }
  }
  if (/^https?:\/\//i.test(value)) return { mime: mime || 'image/png', url: value }
  if (!/^[a-z0-9+/=_-]+$/i.test(value)) return undefined
  const detected = value.startsWith('/9j/')
    ? 'image/jpeg'
    : value.startsWith('R0lGOD')
      ? 'image/gif'
      : value.startsWith('UklGR')
        ? 'image/webp'
        : 'image/png'
  return { mime: mime || detected, url: `data:${mime || detected};base64,${value}` }
}

// Chat and Messages encode media as Markdown, including across SSE boundaries.
export class MarkdownMediaDecoder {
  private pending = ''

  push(text: string, final = false): { text: string; media: PlaygroundMedia[] } {
    this.pending += text
    const media: PlaygroundMedia[] = []
    let output = ''
    while (this.pending) {
      const start = this.pending.indexOf('![')
      if (start < 0) {
        const keep = !final && this.pending.endsWith('!') ? 1 : 0
        output += this.pending.slice(0, this.pending.length - keep)
        this.pending = keep ? '!' : ''
        break
      }
      output += this.pending.slice(0, start)
      this.pending = this.pending.slice(start)
      const match = /^!\[[^\]]*\]\(([^\s)]+)\)/.exec(this.pending)
      if (match) {
        const image = imageMedia(match[1]!)
        if (image) media.push(image)
        else output += match[0]
        this.pending = this.pending.slice(match[0].length)
        continue
      }
      // Incomplete Markdown is kept until the next frame. Malformed/ordinary
      // text is released at completion instead of being silently discarded.
      if (final) {
        output += this.pending
        this.pending = ''
      }
      break
    }
    return { text: output, media }
  }
}
