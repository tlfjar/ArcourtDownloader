# Set up Google (Gemini) API access for AI document names

This guide takes you from a new Gemini API account to a configured
ArcourtDownloader. You do not need to install an SDK or run any code.

**Official links last checked: 2026-11-15.** Consoles, plans, and model lists
change. Where this page and Google's own pages differ, follow Google's.

AI naming is optional and off by default. When enabled, it sends bounded content
from the first two pages of each newly downloaded PDF to Google, and API charges
may apply. Read the [AI naming guide](../ai-document-naming.md) first. Treat your
API key like a password: never put it in an issue, screenshot, or email.

## 1. Open Google AI Studio

Sign in to [Google AI Studio](https://aistudio.google.com/) with a Google
account. Each API key belongs to a Google Cloud project, which AI Studio can
create or import for you. The [API key guide](https://aistudio.google.com/docs/api-key)
describes the current steps.

## 2. Check credits and billing

The API is billed per project, separately from the Gemini apps. A Gemini app or
Google One subscription does not by itself confirm that this app has usable API
access. Confirm the project's tier and billing status in AI Studio.

The [billing page](https://ai.google.dev/gemini-api/docs/billing) explains the
usage tiers, how to link a billing account or buy prepaid credits, and the
monthly spend cap that applies to each paid tier. Which models a project can
call, and at what rate, depends on its current tier. Data-handling terms can
also depend on the tier, so review the terms that apply to yours before sending
court documents.

## 3. Create and copy an API key

On the AI Studio **API Keys** page, select **Create API key** and choose the
project, following the [API key guide](https://aistudio.google.com/docs/api-key).
Copy the key and keep it only until you finish step 5. A key created just for
this app is easy to revoke later without affecting anything else.

## 4. Choose a model ID

The [Gemini API documentation](https://aistudio.google.com/docs) lists a
**Model code** and **Modalities** for each model. Copy the model code exactly,
for example `gemini-4-hydrogen`. Enter the bare code without a `models/` prefix;
the **Model ID** field does not accept a `/`.

- **Route.** The app calls
  `https://generativelanguage.googleapis.com/v1beta/models/{model}:generateContent`.
  Choose a model that produces text, not a speech or image-output model.
- **Inputs.** PDFs with usable text need text input only. Scanned pages are sent
  as PNG or JPEG images and need a model whose modalities include image input.
- **Evaluated or example.** `gemini-4-hydrogen` is only an example of the code
  format. This project has not measured naming quality for any Google model.
  The connector's requests and responses are tested, but every Google model ID
  is experimental. See the [evidence](../ai-document-naming.md#benchmark-and-evidence).

## 5. Enter the values in ArcourtDownloader

1. Open **Settings & diagnostics** and find **Optional AI document names**.
2. Check **Enable AI naming for newly downloaded PDFs**.
3. Set **Provider** to **Google** and paste the model code into **Model ID**.
4. Read the **Direct recipient** line and the disclosure beneath it, then check
   the **I agree to send bounded text or images…** box. Changing the provider
   later clears this box.
5. Select **Save settings**. This validates the entries locally. It does not
   contact Google.
6. Under **Provider API key**, paste the key into **API key** and select
   **Save or replace key**. This is a separate action from **Save settings** and
   is available only after the provider has been saved.

## 6. Check, replace, or remove the key

The status line under **Provider API key** reads **API key configured for this
provider.** once a key is stored. The key itself is never displayed again. To
change it, paste a new key and select **Save or replace key**. To delete it,
select **Remove key**.

The key is stored for your Windows user in Windows Credential Manager, under
**Windows Credentials → Generic Credentials** as `ArcourtDownloader/AI/google`.
It is not written to `settings.json` or to download manifests. Each provider's
key is stored separately. Removing the key here does not revoke it at Google;
delete it on the AI Studio **API Keys** page as well when you stop using it.

## Troubleshooting

The app has no connection test. The first real check is downloading a document
that has not been saved before; already verified files are not renamed. Each
document's result shows **AI label** or **Standard filename (reason)**. With a
standard filename the PDF has still been saved normally.

| What the app shows | What to check |
| --- | --- |
| **No API key saved for this provider.** or **AI naming skipped: provider API key is missing or unavailable.** | Save the key as in step 5. Confirm the saved provider is Google. |
| **AI naming skipped: review and consent to the selected provider in Settings.** | Check the consent box and select **Save settings**. |
| **Windows secure credential storage is unavailable.** | Credential Manager could not be used for this Windows user. Downloads continue with standard filenames. |
| A message that the model ID must be letters, digits, dots, underscores, colons, or hyphens | Remove any `models/` prefix and copy the model code again, without spaces or other characters. |
| **Standard filename (provider rejected the API key)** | The key lacks permission, or the project is restricted or needs a billing change. Check the project's status, then create a new key and replace it if needed. |
| **Standard filename (model unavailable)** | The model code is misspelled, retired, or not available to your project. Copy it again from the documentation. |
| **Standard filename (provider rate limit)** or **(provider error)** | Check the project's tier, credit balance, and rate limits, then retry later. Google can report an invalid key or exhausted credits with a general error, which the app shows as **provider error**. See Google's [troubleshooting guide](https://ai.google.dev/gemini-api/docs/troubleshooting). |
| **Standard filename (provider error)** or **(model unavailable)** on scanned documents only | The model may not accept image input. Choose a model whose modalities include image input. |
| **Standard filename (scanned page image unavailable)** | The app could not prepare a usable page image locally, so nothing was sent. This is not an account or model problem. |

The app never switches providers or retries with a different model on its own.
The [AI naming guide](../ai-document-naming.md#results-shown-in-the-desktop)
lists every result and reason.
