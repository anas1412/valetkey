# Using a key in your app

Valetkey speaks the OpenAI Chat Completions API and the Anthropic Messages API. Point your SDK at Valetkey and use the virtual key. Nothing else changes.

Inside the cluster the endpoint is `http://valetkey.valetkey-system:8000`. (It's the `VALETKEY_ENDPOINT` value in the key's Secret.)

## OpenAI SDKs

::: code-group

```python [Python]
from openai import OpenAI

client = OpenAI(base_url="http://valetkey.valetkey-system:8000/v1", api_key="sk-proxy-…")
reply = client.chat.completions.create(
    model="gpt-4o-mini",
    messages=[{"role": "user", "content": "Hello"}],
)
print(reply.choices[0].message.content)
```

```ts [TypeScript]
import OpenAI from "openai";

const client = new OpenAI({ baseURL: "http://valetkey.valetkey-system:8000/v1", apiKey: "sk-proxy-…" });
const reply = await client.chat.completions.create({
  model: "gpt-4o-mini",
  messages: [{ role: "user", content: "Hello" }],
});
console.log(reply.choices[0].message.content);
```

```bash [cURL]
curl http://valetkey.valetkey-system:8000/v1/chat/completions \
  -H "Authorization: Bearer sk-proxy-…" \
  -H "Content-Type: application/json" \
  -d '{"model":"gpt-4o-mini","messages":[{"role":"user","content":"Hello"}]}'
```

:::

Streaming (`"stream": true`), tools and images work as usual. The same OpenAI code also works with **Anthropic** models when the tenant has an Anthropic provider; Valetkey translates both ways.

## Anthropic SDKs

```python
import anthropic

client = anthropic.Anthropic(base_url="http://valetkey.valetkey-system:8000", api_key="sk-proxy-…")
msg = client.messages.create(model="claude-sonnet-4-5", max_tokens=256,
                             messages=[{"role": "user", "content": "Hello"}])
print(msg.content[0].text)
```

`/v1/messages` is passed through to Anthropic providers unchanged.

## Which models can a key use?

```bash
curl http://valetkey.valetkey-system:8000/v1/models -H "Authorization: Bearer sk-proxy-…"
```

The list is the tenant's providers' models, narrowed to the key's allowed models.

## Errors your app may see

| Status | Meaning |
|---|---|
| 401 | Missing or unknown key, or the client IP isn't in the key's allowed ranges |
| 403 | The key may not use this model |
| 429 | The key's requests-per-minute or tokens-per-minute limit is reached (see `Retry-After`) |
| 503 | No enabled provider in the tenant serves this model |
| 502 / 504 | Every provider failed or timed out |

Errors use the same JSON shape as the API you called (OpenAI or Anthropic), so SDKs report them normally.
