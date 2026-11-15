# Set up OpenAI API access for AI document names

This guide takes you from a new OpenAI API account to a configured
ArcourtDownloader. You do not need to install an SDK or run any code.

**Official links last checked: 2026-11-15.** Consoles, plans, and model lists
change. Where this page and OpenAI's own pages differ, follow OpenAI's.

AI naming is optional and off by default. When enabled, it sends bounded content
from the first two pages of each newly downloaded PDF to OpenAI, and API charges
may apply. Read the [AI naming guide](../ai-document-naming.md) first. Treat your
API key like a password: never put it in an issue, screenshot, or email.

## 1. Open the API platform

Sign in or create an account at the [OpenAI API platform](https://platform.openai.com/).
API work is grouped into an organization and projects; the default project is
enough for this app. The [developer quickstart](https://developers.openai.com/api/docs/quickstart)
and the [projects article](https://help.openai.com/en/articles/9186755-managing-your-work-in-the-api-platform-with-projects)
describe the current layout.

## 2. Check credits and billing

The API is billed separately from the ChatGPT apps. A ChatGPT subscription does
not by itself confirm that this app has usable API access. Some plans can claim
API credits; see [API credits for ChatGPT subscribers](https://developers.openai.com/api/docs/guides/chatgpt-api-credits).
Either way, confirm a usable balance on the platform's billing overview.

[Setting up prepaid API billing](https://help.openai.com/en/articles/8264644-setting-up-and-managing-prepaid-api-billing)
explains buying credits. Review the auto-reload option during setup so that
automatic purchases match what you intend. The platform's usage dashboard shows
consumption. When checked, a project's monthly spend limit sent an alert rather
than stopping requests, so do not rely on it as a hard cap.

## 3. Create and copy an API key

Create a key on the project's **API keys** page, following the
[quickstart](https://developers.openai.com/api/docs/quickstart). Copy it when it
is shown and keep it only until you finish step 5. A key created and named just
for this app is easy to revoke later without affecting anything else.

## 4. Choose a model ID

The [models page](https://developers.openai.com/api/docs/models) lists each
**Model ID**. Copy it exactly, for example `gpt-6-luna`.

- **Route.** The app calls the Responses API at `https://api.openai.com/v1/responses`.
  Choose a text-generating model offered there, not an audio, realtime,
  image-generation, or embedding model.
- **Inputs.** PDFs with usable text need text input only. Scanned pages are sent
  as PNG or JPEG images and need a model that accepts image input. The models
  page shows each model's supported inputs.
- **Evaluated or example.** `gpt-6-luna` is the only model whose text-excerpt
  naming quality this project has measured. Its scanned-page image naming, and
  every other OpenAI model ID, are unmeasured and experimental even though the
  connector supports them. See the [evidence](../ai-document-naming.md#benchmark-and-evidence).

## 5. Enter the values in ArcourtDownloader

1. Open **Settings & diagnostics** and find **Optional AI document names**.
2. Check **Enable AI naming for newly downloaded PDFs**.
3. Set **Provider** to **OpenAI** and paste the model ID into **Model ID**.
4. Read the **Direct recipient** line and the disclosure beneath it, then check
   the **I agree to send bounded text or images…** box. Changing the provider
   later clears this box.
5. Select **Save settings**. This validates the entries locally. It does not
   contact OpenAI.
6. Under **Provider API key**, paste the key into **API key** and select
   **Save or replace key**. This is a separate action from **Save settings** and
   is available only after the provider has been saved.

## 6. Check, replace, or remove the key

The status line under **Provider API key** reads **API key configured for this
provider.** once a key is stored. The key itself is never displayed again. To
change it, paste a new key and select **Save or replace key**. To delete it,
select **Remove key**.

The key is stored for your Windows user in Windows Credential Manager, under
**Windows Credentials → Generic Credentials** as `ArcourtDownloader/AI/openai`.
It is not written to `settings.json` or to download manifests. Each provider's
key is stored separately. Removing the key here does not revoke it at OpenAI;
delete it on the platform's **API keys** page as well when you stop using it.

## Troubleshooting

The app has no connection test. The first real check is downloading a document
that has not been saved before; already verified files are not renamed. Each
document's result shows **AI label** or **Standard filename (reason)**. With a
standard filename the PDF has still been saved normally.

| What the app shows | What to check |
| --- | --- |
| **No API key saved for this provider.** or **AI naming skipped: provider API key is missing or unavailable.** | Save the key as in step 5. Confirm the saved provider is OpenAI. |
| **AI naming skipped: review and consent to the selected provider in Settings.** | Check the consent box and select **Save settings**. |
| **Windows secure credential storage is unavailable.** | Credential Manager could not be used for this Windows user. Downloads continue with standard filenames. |
| A message that the model ID must be letters, digits, dots, underscores, colons, or hyphens | Copy the ID again from the models page, without spaces or other characters. |
| **Standard filename (provider rejected the API key)** | The key is wrong, revoked, or lacks access to the project or model. Create a new key and replace it. |
| **Standard filename (model unavailable)** | The model ID is misspelled, retired, or not available to your account. Copy it again from the models page. |
| **Standard filename (provider rate limit)** or **(provider error)** | Check the credit balance, billing status, and limits on the platform, then retry later. Depending on how the provider reports it, an exhausted balance can appear under either reason. |
| **Standard filename (provider error)** or **(model unavailable)** on scanned documents only | The model may not accept image input. Choose an image-capable model. |
| **Standard filename (scanned page image unavailable)** | The app could not prepare a usable page image locally, so nothing was sent. This is not an account or model problem. |

The app never switches providers or retries with a different model on its own.
The [AI naming guide](../ai-document-naming.md#results-shown-in-the-desktop)
lists every result and reason.
