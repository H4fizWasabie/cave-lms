# OpenRouter's handling of data we send it

Research for #208 (map #205). The question: if Cave LMS sends a Learner's spreadsheet column names and about 20 sample rows (Malaysian office staff's business data, which may include names or amounts) to OpenRouter's chat completions API for `openai/gpt-6-luna`, what happens to that data at OpenRouter and at the model provider?

Researched 24 Oct 2026 against primary sources. OpenRouter's docs pages show no "last updated" date. The live API listings were read on that day and change when providers change.

## At OpenRouter

- **Prompts are not stored by default.** OpenRouter "does not store your prompts or responses, *unless* you opt in" to one of two settings. Both are off by default: Private Input & Output Logging (Observability settings) and "OpenRouter Use of Inputs/Outputs" (Privacy settings, gives a 1% discount). Source: https://openrouter.ai/docs/guides/privacy/data-collection.md. The ZDR page says the same: OpenRouter itself has a ZDR policy and prompts are "not retained unless you specifically opt in to prompt logging" (https://openrouter.ai/docs/guides/features/zdr.md).
- **There is one exception.** OpenRouter "samples a small number of prompts for categorization". If you are not opted in, that categorization is "stored completely anonymously and never associated with your account", and a ZDR model does the categorizing (data-collection page). The Terms say Inputs are not stored after categorization unless you opt into logging (https://openrouter.ai/terms, Last updated 31 Aug 2026).
- **Metadata is always stored.** That means token counts, latency and similar, which feed the logs page and rankings. It "does not include the content of your prompts or responses" (data-collection page).
- **If logging is opted in, content is kept a long time.** It is kept in Google Cloud Storage for "a minimum of 3 months, and may be retained beyond 3 months at OpenRouter's discretion" unless deletion is requested. Org admins can read all logged content (https://openrouter.ai/docs/guides/features/input-output-logging.md). If the logging licence is opted in, the Terms widen to a perpetual licence that includes selling content "in anonymized form" (Terms §6). **So both opt-in toggles must stay off on the Cave LMS account.**
- **OpenRouter does not train on prompts.** The Privacy Policy (Last Updated 31 Aug 2026, https://openrouter.ai/privacy) says "OpenRouter does not use your Inputs or Outputs for model training." It also says Inputs that contain personal data "will be collected by us", that data may go to US servers and other countries outside the EEA/UK under SCCs, and that it is kept "as long as is reasonably necessary". The policy gives no fixed retention period for text Inputs.
- **A DPA is available.** The Terms §10.2 offer a Data Processing Agreement for organizations and commercial use, "incorporated by reference" into the Terms.
- **OpenRouter forwards the request's country of origin to models** on a best-effort basis (Terms).

## At the model provider: who serves `openai/gpt-6-luna`

Sources: https://openrouter.ai/api/v1/models/openai/gpt-6-luna/endpoints for the endpoints and prices, and the model's providers page https://openrouter.ai/openai/gpt-6-luna/providers for the per-endpoint `data_policy` in its page data. Prices are USD per million tokens, input/output.

| Endpoint slug | Provider | Price in/out | Retains prompts | Trains | On ZDR list |
| - | - | - | - | - | - |
| `openai` | OpenAI | 0.10 / 0.50 | yes (no period stated) | no | no |
| `openai/flex` (service tier) | OpenAI | 0.05 / 0.25 | yes | no | no |
| `openai/fast` (priority tier) | OpenAI | 0.20 / 1.00 | yes | no | no |
| `azure` | Azure (base URL `openrouter-east-us-2.openai.azure.com`) | 0.10 / 0.50 | no | no | **yes** |
| `azure/us` | Azure | 0.11 / 0.55 | no | no | **yes** |
| `azure/eu` | Azure | 0.11 / 0.55 | no | no | **yes** |
| `amazon-bedrock/us-east-1` | Amazon Bedrock | 0.11 / 0.55 | yes, `retentionDays: 30` | no | no |

There is also a batch variant (`openai/gpt-6-luna:batch`, 0.05 / 0.25) whose OpenAI endpoint retains for 30 days.

- **Only the Azure endpoints are ZDR.** The ZDR endpoint list (https://openrouter.ai/api/v1/endpoints/zdr) contains exactly `azure`, `azure/us` and `azure/eu` for `openai/gpt-6-luna`. All three list `response_format` and `structured_outputs` in `supported_parameters`, so the existing strict JSON-schema calls should still work. One difference is unverified: Azure lists `max_completion_tokens` rather than `max_tokens`, and `internal/aiwriter/openrouter.go` sends `max_tokens`. Test one call before relying on it.
- **No endpoint trains.** Every gpt-6-luna endpoint has `training: false`, so the endpoints differ on retention, not training. The docs do not say clearly whether `data_collection: "deny"` also drops endpoints that retain but do not train. `zdr` is the documented control for retention.
- **OpenAI's own API policy** (https://developers.openai.com/api/docs/guides/your-data, the target of a 301 from platform.openai.com; no date shown): API data is "not used to train or improve OpenAI models" unless you opt in. Abuse-monitoring logs for `/v1/chat/completions` are "retained for up to 30 days, unless longer retention is required by law". OpenAI's own ZDR and Modified Abuse Monitoring require sales approval. That approval is for a direct OpenAI customer and does not reach requests made through OpenRouter's key.
- **Azure's own policy** (https://learn.microsoft.com/en-us/azure/ai-foundry/responsible-ai/openai/data-privacy, ms.date 18 May 2026, change log last entry 3 Oct 2025): prompts are "NOT available to OpenAI" and are not used to train foundation models. By default Azure keeps prompts that abuse monitoring flags, for human review, in the resource's geography. Approved customers can turn this "modified abuse monitoring" off, and automated review still runs. OpenRouter marks its Azure endpoints `retainsPrompts: false`, which suggests OpenRouter's Azure resources have that exemption (the ZDR page says it makes "special agreements with providers"). OpenRouter does not publish that agreement, so this is an inference.
- **Unknown policies count as retaining.** If OpenRouter cannot establish a policy, it "assume[s] that the endpoint both retains and trains on data" (ZDR page).
- **Two more flags appear on OpenAI's endpoints.** Each carries `requiresUserIDs: true` and `moderation_required: true` in its page data. OpenRouter does not explain these on the pages read.

## Requiring ZDR or no-training

These live in the `provider` object of the chat completions request body (https://openrouter.ai/docs/guides/routing/provider-selection.md):

- **`"provider": {"zdr": true}`** routes "only to endpoints that have a Zero Data Retention policy". It combines with account and guardrail settings as an OR: it can switch ZDR on for one request but cannot switch an account-level setting off.
- **`"provider": {"data_collection": "deny"}`** (default `"allow"`) means "use only providers which do not collect user data". OpenRouter's docs tie this to providers that store data and may train on it.
- **Account level:** https://openrouter.ai/settings/privacy has per-model-group ZDR toggles. The groups are Anthropic, OpenAI, Google, SpaceXAI and all other models. The OpenAI toggle "Removes first-party OpenAI endpoints (Azure remains available)". There is also a separate setting to block providers that train, with separate switches for paid and free models (https://openrouter.ai/docs/guides/privacy/provider-logging.md).
- **Guardrail level (per API key):** the fields are `enforce_zdr_openai`, `enforce_zdr_anthropic`, `enforce_zdr_google`, `enforce_zdr_xai` and `enforce_zdr_other`. The older `enforce_zdr` is deprecated.
- **What ZDR does not cover:**
  - Plugins and server tools are not covered: "ZDR enforcement only applies to provider routing for inference requests." This matters because `WriteQuestions` adds the `web` plugin when `OPENROUTER_WEB_SEARCH` is on. When ZDR is enforced at account or guardrail level, the web tools reject the Firecrawl engine (ZDR page).
  - In-memory prompt caching does not count as retention, so it stays allowed under ZDR (ZDR page).
- **If no ZDR endpoint is up:** the ZDR page does not say what happens. The in-region page says regional routing errors rather than leaving the region, and ZDR filtering presumably also ends in a no-endpoint error. This was not tested.

## Data location

- **OpenRouter itself:** the US and other non-EEA countries (Privacy Policy).
- **In-region routing:** only `eu.openrouter.ai` and `us.openrouter.ai`, and only on the Business and Enterprise plans. It "fails closed" and never routes a regional request outside the region (https://openrouter.ai/docs/guides/features/in-region-routing.md). There is no Asia or Malaysia region. Input/output logging is skipped on the regional domains.
- **gpt-6-luna:**
  - The default ZDR endpoint `azure` runs in Azure East US 2 (from its base URL).
  - `azure/eu` is in the EU. On the global domain it is a regional-surcharge endpoint, so default routing skips it unless it is pinned with `provider.only`/`order` or the request goes through `eu.openrouter.ai` (in-region page).
  - Azure keeps abuse-monitoring data in the resource's geography (Azure page).
- **OpenAI's data residency** offers US, EU, AU, CA, JP, IN, SG, KR, GB and AE, with no Malaysia. That is for direct OpenAI customers, not for OpenRouter traffic (OpenAI your-data page).
- Malaysia's PDPA rules on cross-border transfer were not researched here.

## Cost and availability when ZDR is required

- **Price:** on the global domain, ZDR routing lands on `azure` at the same standard price as `openai` (0.10 / 0.50). Requiring ZDR adds no cost for standard calls. It does rule out the cheaper OpenAI `flex` tier and the `:batch` variant (0.05 / 0.25), and the `fast` tier. Pinning `azure/eu` costs about 10% more.
- **Availability:** ZDR shrinks the pool from 7 endpoints across 3 providers to one provider, Azure, with 3 endpoints, of which only `azure` is used by default. Fallbacks to OpenAI or Bedrock are gone. On 24 Oct 2026 Azure showed 100% uptime over the last day. One provider remains a single point of failure.
- **Features:** a model's ZDR endpoints may support fewer parameters. For gpt-6-luna on Azure the list is close to OpenAI's but not identical (`max_completion_tokens`, see above).

## What this means for the decision (#212)

These are options; the decision belongs to #212.

1. **Send without ZDR (status quo routing).** OpenRouter keeps nothing beyond metadata and an anonymous categorization sample. No endpoint trains. OpenAI and Bedrock may keep prompts for up to 30 days for abuse monitoring. It costs least and has the most fallbacks.
2. **Send with `"provider": {"zdr": true}` on the spreadsheet request only.** It goes to Azure at the same price, with no provider retention and no training, but in the US and with Azure as the only provider. This is a one-field change in the request body, with no web plugin on that call.
3. **Add a guardrail or account-level `enforce_zdr_openai` as well.** This makes ZDR hold for every call on the key, Exam writing and marking included. It also stops Firecrawl-backed web search, which matters for question writing with web search on.
4. **Use ZDR plus EU residency** through `azure/eu` (about 10% more) or the Business plan's `eu.openrouter.ai`. This only matters if the client wants a named region. Malaysia is not on offer.
5. **Reduce or redact what is sent:** column names only, or masked or synthetic sample rows. This works with any of the above, lowers the stakes whatever the provider does, and fits ADR 0009's rule of sending only what the task needs.

Whichever option is chosen, keep both OpenRouter opt-in logging toggles off and confirm that in the deploy notes. Like ADR 0009, the privacy notice will likely need a line that the client approves.
