// Thin re-exports that pin Wails bindings to short names the React tree uses.
// Centralising here keeps generated import paths out of every component.

import { Service as Accounts } from "../bindings/github.com/mandloideep/miniclaw/internal/services/account";
import { Service as Digest } from "../bindings/github.com/mandloideep/miniclaw/internal/services/digest";
import {
  IMAPSyncer as IMAPSync,
  SMTPSender,
} from "../bindings/github.com/mandloideep/miniclaw/internal/services/email";
import {
  Service as GmailAuth,
  Syncer as GmailSync,
} from "../bindings/github.com/mandloideep/miniclaw/internal/services/gmailoauth";
import { Service as Inbox } from "../bindings/github.com/mandloideep/miniclaw/internal/services/inbox";
import { Service as Keychain } from "../bindings/github.com/mandloideep/miniclaw/internal/services/keychain";
import {
  Service as MSAuth,
  Syncer as MSSync,
} from "../bindings/github.com/mandloideep/miniclaw/internal/services/msoauth";
import { Service as Ollama } from "../bindings/github.com/mandloideep/miniclaw/internal/services/ollama";
import { Service as Telegram } from "../bindings/github.com/mandloideep/miniclaw/internal/services/telegram";
import { Service as Triage } from "../bindings/github.com/mandloideep/miniclaw/internal/services/triage";
import { Service as Workspaces } from "../bindings/github.com/mandloideep/miniclaw/internal/services/workspace";

// GmailOAuth surface combines the auth methods (StartAuthorize) and the
// sync methods (SyncNow, BackfillBefore) so the UI only deals with one name.
const GmailOAuth = { ...GmailAuth, ...GmailSync };
const MSOAuth = { ...MSAuth, ...MSSync };

export {
  Accounts,
  Digest,
  GmailOAuth,
  IMAPSync,
  Inbox,
  Keychain,
  MSOAuth,
  Ollama,
  SMTPSender,
  Telegram,
  Triage,
  Workspaces,
};
