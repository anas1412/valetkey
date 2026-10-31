# Limits & access rules

Every request is checked in this order:

1. **Key**: known and enabled, tenant enabled.
2. **Client IP**: inside the key's allowed ranges (if any).
3. **Model**: in the key's allowed models (if any), and served by one of the tenant's providers.
4. **Rate limits**: under the key's requests-per-minute and tokens-per-minute.

## Allowed models

Set per key (and per provider). A key without a model list can use every model its tenant's providers offer.

## Allowed IP ranges

```yaml
spec:
  subnets: [10.0.0.0/8, 2001:db8::/32, 192.168.1.5]
```

IPv4 and IPv6 ranges, or single addresses. Valetkey checks the address the connection comes from.

::: warning Behind an ingress or load balancer
The connection then comes from the ingress, not the client. Set [`TRUST_PROXY`](/reference/configuration) to the number of proxies in front of Valetkey so the client address is read from `X-Forwarded-For`. For a `LoadBalancer` Service, `externalTrafficPolicy: Local` keeps the client address.
:::

## Rate limits

```yaml
spec:
  rpm: 60        # requests per minute
  tpm: 100000    # tokens per minute
```

The tenant's `rpmRatio` and `tpmRatio` multiply these. When a limit is hit, the app gets **429** with a `Retry-After` header.

Token counts come from the provider's usage data. For OpenAI-style streaming, the client must request it with `stream_options: {"include_usage": true}` for tokens to be counted.

## Scaling

Run **one replica**. Limits and dashboard history are counted in memory per pod, and the key reconciler has no leader election yet. One pod handles a lot of traffic: the work per request is small next to the provider's response time.
