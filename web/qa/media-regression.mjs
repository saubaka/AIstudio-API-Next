/* global URL, Response, AbortController, console */
// Exercise the actual browser request/parser code with protocol responses.
import assert from 'node:assert/strict'
import { mkdtemp, readFile, writeFile, rm } from 'node:fs/promises'
import { fileURLToPath, pathToFileURL } from 'node:url'
import ts from 'typescript'
const temp = await mkdtemp(fileURLToPath(new URL('../.media-test-', import.meta.url)))
const originalFetch = globalThis.fetch
try {
  for (const name of ['playground-media', 'api']) {
    const source = await readFile(new URL(`../src/${name}.ts`, import.meta.url), 'utf8')
    const compiled = ts.transpileModule(source, {
      compilerOptions: { target: ts.ScriptTarget.ESNext, module: ts.ModuleKind.ESNext },
    }).outputText
    await writeFile(
      `${temp}/${name}.mjs`,
      compiled.replace("'./playground-media'", "'./playground-media.mjs'"),
    )
  }
  const { runPlayground } = await import(pathToFileURL(`${temp}/api.mjs`))
  const { MarkdownMediaDecoder } = await import(pathToFileURL(`${temp}/playground-media.mjs`))
  const png =
    'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+a4T8AAAAASUVORK5CYII='
  const url = `data:image/png;base64,${png}`
  const markdown = `before ![media](${url}) after`
  // Every possible two-frame split, including inside Base64 and Markdown markers.
  for (let split = 0; split < markdown.length; split++) {
    const d = new MarkdownMediaDecoder()
    const a = d.push(markdown.slice(0, split))
    const b = d.push(markdown.slice(split), true)
    assert.equal(a.text + b.text, 'before  after')
    assert.deepEqual([...a.media, ...b.media], [{ mime: 'image/png', url }])
  }
  const incomplete = new MarkdownMediaDecoder()
  assert.equal(incomplete.push('ordinary ![unfinished', true).text, 'ordinary ![unfinished')
  const unsafe = new MarkdownMediaDecoder()
  assert.equal(unsafe.push('![x](javascript:alert)', true).media.length, 0)
  const base = {
    mode: 'text',
    model: 'image-model',
    prompt: 'Draw a circle',
    apiKey: '',
    system: '',
    tool: '',
    reasoning: '',
    imageSize: 'auto',
    imageQuality: 'auto',
    voice: '',
  }
  const cases = [
    ['openai-chat', false, { choices: [{ message: { content: markdown } }] }],
    [
      'openai-chat',
      false,
      {
        choices: [
          {
            message: {
              content: [
                { type: 'text', text: 'before' },
                { type: 'image_url', image_url: { url } },
              ],
            },
          },
        ],
      },
    ],
    ['anthropic', false, { content: [{ type: 'text', text: markdown }] }],
    ['openai-responses', false, { output: [{ type: 'image_generation_call', result: png }] }],
    [
      'gemini',
      false,
      {
        candidates: [
          { content: { parts: [{ inlineData: { mimeType: 'image/png', data: png } }] } },
        ],
      },
    ],
    [
      'gemini',
      false,
      {
        candidates: [
          { content: { parts: [{ inline_data: { mime_type: 'image/png', data: png } }] } },
        ],
      },
    ],
    [
      'openai-chat',
      true,
      [
        ...['before !', '[media](', url.slice(0, 55), url.slice(55), ') after'].map((content) => ({
          choices: [{ delta: { content } }],
        })),
        '[DONE]',
      ],
    ],
    [
      'anthropic',
      true,
      [{ type: 'content_block_delta', delta: { text: markdown } }, { type: 'message_stop' }],
    ],
    [
      'openai-responses',
      true,
      [
        {
          type: 'response.output_item.added',
          item: { type: 'image_generation_call', result: null },
        },
        { type: 'response.output_item.done', item: { type: 'image_generation_call', result: png } },
        {
          type: 'response.completed',
          response: { output: [{ type: 'image_generation_call', result: png }] },
        },
      ],
    ],
    [
      'gemini',
      true,
      [
        {
          candidates: [
            {
              content: { parts: [{ inlineData: { mimeType: 'image/png', data: png } }] },
              finishReason: 'STOP',
            },
          ],
        },
      ],
    ],
  ]
  let count = 0
  for (const channel of ['playground', 'build'])
    for (const [protocol, stream, value] of cases) {
      globalThis.fetch = async (path) => {
        assert.ok(path.startsWith(`/${channel}/`))
        return new Response(
          stream
            ? value
                .map((x) => `data: ${typeof x === 'string' ? x : JSON.stringify(x)}\n\n`)
                .join('')
            : JSON.stringify(value),
          { headers: { 'content-type': stream ? 'text/event-stream' : 'application/json' } },
        )
      }
      const chunks = []
      const response = await runPlayground(
        { ...base, channel, protocol, stream },
        new AbortController().signal,
        (c) => chunks.push(c),
      )
      if (response.chunk) chunks.push(response.chunk)
      const media = chunks.flatMap((c) => c.media)
      assert.deepEqual(media, [{ mime: 'image/png', url }])
      assert.ok(
        !chunks
          .map((c) => c.text)
          .join('')
          .includes(png),
      )
      count++
    }
  for (const value of [{ data: [{ url }] }, { data: [{ b64_json: png }] }]) {
    globalThis.fetch = async () => new Response(JSON.stringify(value))
    const r = await runPlayground(
      { ...base, channel: 'build', mode: 'image', protocol: 'openai-chat', stream: false },
      new AbortController().signal,
      () => {},
    )
    assert.deepEqual(r.chunk.media, [{ mime: 'image/png', url }])
    count++
  }
  console.log(
    `PASSED: ${count} protocol/channel media cases and ${markdown.length} SSE split boundaries`,
  )
} finally {
  globalThis.fetch = originalFetch
  await rm(temp, { recursive: true, force: true })
}
