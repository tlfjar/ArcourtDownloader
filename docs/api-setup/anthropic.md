# Set up Anthropic (Claude) API access for AI document names

This guide takes you from a new Claude API account to a configured
ArcourtDownloader. You do not need to install an SDK or run any code.

**Official links last checked: 2026-11-15.** Consoles, plans, and model lists
change. Where this page and Anthropic's own pages differ, follow Anthropic's.

AI naming is optional and off by default. When enabled, it sends bounded content
from the first two pages of each newly downloaded PDF to Anthropic, and API
charges may apply. Read the [AI naming guide](../ai-document-naming.md) first.
Treat your API key like a password: never put it in an issue, screenshot, or
email.

## 1. Open the Claude Console

Sign in or create an account at the [Claude Console](https://platform.claude.com).
The Console groups keys and billing under an organization, with optional
workspaces; the defaults are enough for this app. The
[API key guide](https://platform.claude.com/docs/en/get-api-key) describes the
current steps.

## 2. Check credits and billing

The API is billed through the Console, separately from the Claude apps. A Claude
subscription does not by itself confirm that this app has usable API access.
Some plans include API credits that must be claimed by linking a Console
organization; see [How do I pay for my Claude API usage?](https://support.claude.com/en/articles/8977456-how-do-i-pay-for-my-claude-api-usage)
Either way, confirm a usable balance on the Console's
[Billing page](https://platform.claude.com/settings/billing).

That article explains buying prepaid usage credits. The Billing page shows the
available balance and credit usage. Check the Console settings for any spend
limits your organization can set.

## 3. Create and copy an API key

On the Console's [API keys page](https://platform.claude.com/settings/keys),
create a key and give it a name, following the
[API key guide](https://platform.claude.com/docs/en/get-api-key). The full key,
which starts with `sk-ant-`, is shown only once. Copy it then and keep it only
until you finish step 5. A key created and named just for this app is easy to
revoke later without affecting anything else.

## 4. Choose a model ID

The [models overview](https://platform.claude.com/docs/en/models/overview) lists
the **Claude API ID** for each model. Copy that ID exactly, not the display
name. This page gives no example ID because the overview is the current source.

- **Route.** The app calls the Messages API at `https://api.anthropic.com/v1/messages`.
  Use the Claude API ID, not an ID listed for another cloud platform.
- **Inputs.** PDFs with usable text need text input only. Scanned pages are sent
  as PNG or JPEG images and need a model that accepts image input. The overview
  shows each model's supported inputs.
- **Evaluated or example.** This project has not measured naming quality for any
  Anthropic model. The connector's requests and responses are tested, but every
  Anthropic model ID is experimental. The overview describes which models suit
  short, high-volume tasks such as this one.
  See the [evidence](../ai-document-naming.md#benchmark-and-evidence).

## 5. Enter the values in ArcourtDownloader

1. Open **Settings & diagnostics** and find **Optional AI document names**.
2. Check **Enable AI naming for newly downloaded PDFs**.
3. Set **Provider** to **Anthropic** and paste the model ID into **Model ID**.
4. Read the **Direct recipient** line and the disclosure beneath it, then check
   the **I agree to send bounded text or images…** box. Changing the provider
   later clears this box.
5. Select **Save settings**. This validates the entries locally. It does not
   contact Anthropic.
6. Under **Provider API key**, paste the key into **API key** and select
   **Save or replace key**. This is a separate action from **Save settings** and
   is available only after the provider has been saved.

## 6. Check, replace, or remove the key

The status line under **Provider API key** reads **API key configured for this
provider.** once a key is stored. The key itself is never displayed again. To
change it, paste a new key and select **Save or replace key**. To delete it,
select **Remove key**.

The key is stored for your Windows user in Windows Credential Manager, under
**Windows Credentials → Generic Credentials** as `ArcourtDownloader/AI/anthropic`.
It is not written to `settings.json` or to download manifests. Each provider's
key is stored separately. Removing the key here does not revoke it at Anthropic;
delete it on the Console's API keys page as well when you stop using it.

## Troubleshooting

The app has no connection test. The first real check is downloading a document
that has not been saved before; already verified files are not renamed. Each
document's result shows **AI label** or **Standard filename (reason)**. With a
standard filename the PDF has still been saved normally.

| What the app shows | What to check |
| --- | --- |
| **No API key saved for this provider.** or **AI naming skipped: provider API key is missing or unavailable.** | Save the key as in step 5. Confirm the saved provider is Anthropic. |
| **AI naming skipped: review and consent to the selected provider in Settings.** | Check the consent box and select **Save settings**. |
| **Windows secure credential storage is unavailable.** | Credential Manager could not be used for this Windows user. Downloads continue with standard filenames. |
| A message that the model ID must be letters, digits, dots, underscores, colons, or hyphens | Copy the Claude API ID again from the models overview, without spaces or other characters. |
| **Standard filename (provider rejected the API key)** | The key is wrong, expired, revoked, or lacks access to the workspace or model. Create a new key and replace it. |
| **Standard filename (model unavailable)** | The model ID is misspelled, retired, or not available to your account. Copy it again from the models overview. |
| **Standard filename (provider rate limit)** or **(provider error)** | Check the credit balance and limits in the Console, then retry later. Depending on how the provider reports it, an exhausted balance can appear under either reason. |
| **Standard filename (provider error)** or **(model unavailable)** on scanned documents only | The model may not accept image input. Choose an image-capable model. |
| **Standard filename (scanned page image unavailable)** | The app could not prepare a usable page image locally, so nothing was sent. This is not an account or model problem. |

The app never switches providers or retries with a different model on its own.
The [AI naming guide](../ai-document-naming.md#results-shown-in-the-desktop)
lists every result and reason.
