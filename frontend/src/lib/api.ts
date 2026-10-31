import { Events } from '@wailsio/runtime'
import * as BackupService from '../../bindings/nooboo/services/backupservice'
import * as ConfigService from '../../bindings/nooboo/services/configservice'
import * as ExceptionService from '../../bindings/nooboo/services/exceptionservice'
import * as ModsService from '../../bindings/nooboo/services/modsservice'
import * as PackService from '../../bindings/nooboo/services/packservice'
import * as TrayService from '../../bindings/nooboo/services/trayservice'
import * as UpdateService from '../../bindings/nooboo/services/updateservice'
import type {
  AppInfo,
  Backup,
  DuplicateSet,
  Exception,
  GameFolder,
  InstallResult,
  Mod,
  Pack,
  Progress,
  Quarantine,
  Storage,
  TidyResult,
  TrayItem,
  UpdateInfo,
} from '../../bindings/nooboo/models/models'

export type {
  AppInfo,
  Backup,
  DuplicateSet,
  Exception,
  GameFolder,
  InstallResult,
  Mod,
  Pack,
  Progress,
  Quarantine,
  Storage,
  TidyResult,
  TrayItem,
  UpdateInfo,
}

export type Kind = 'mod' | 'cc'

export interface InstallRequest {
  source: string
  name: string
  creator: string
  kind: Kind
}

/**
 * The only place the generated bindings are touched.
 * Screens call this, so a bindings change stays a one-file diff.
 */
export const api = {
  current: () => ConfigService.Current() as unknown as Promise<GameFolder>,
  detectGameRoot: () => ConfigService.DetectGameRoot() as unknown as Promise<string>,
  pickGameFolder: () => ConfigService.PickGameFolder() as unknown as Promise<GameFolder>,
  setGameRoot: (root: string) => ConfigService.SetGameRoot(root) as unknown as Promise<GameFolder>,
  reveal: (path: string) => ConfigService.Reveal(path) as unknown as Promise<void>,
  keepLast: () => ConfigService.KeepLast() as unknown as Promise<number>,
  setKeepLast: (n: number) => ConfigService.SetKeepLast(n) as unknown as Promise<void>,
  about: () => ConfigService.About() as unknown as Promise<AppInfo>,
  openRepo: () => ConfigService.OpenRepo() as unknown as Promise<void>,
  openRelease: (url: string) => ConfigService.OpenRelease(url) as unknown as Promise<void>,
  checkUpdate: () => UpdateService.Check() as unknown as Promise<UpdateInfo>,

  scan: () => ModsService.Scan() as unknown as Promise<Mod[]>,
  setEnabled: (ids: string[], enabled: boolean) =>
    ModsService.SetEnabled(ids, enabled) as unknown as Promise<void>,
  pickFile: () => ModsService.PickFile() as unknown as Promise<string>,
  pickFolder: () => ModsService.PickFolder() as unknown as Promise<string>,
  install: (req: InstallRequest) =>
    ModsService.InstallDownload(req as never) as unknown as Promise<InstallResult>,
  deleteMod: (id: string) => ModsService.Delete(id) as unknown as Promise<void>,
  tidy: () => ModsService.Tidy() as unknown as Promise<TidyResult>,
  thumbnail: (id: string) => ModsService.Thumbnail(id) as unknown as Promise<string>,
  findDuplicates: () => ModsService.FindDuplicates() as unknown as Promise<DuplicateSet[]>,
  setAsideFile: (file: string) => ModsService.SetAsideFile(file) as unknown as Promise<void>,
  ignoreConflict: (a: string, b: string) =>
    ModsService.IgnoreConflict(a, b) as unknown as Promise<void>,
  addTag: (id: string, tag: string) => ModsService.AddTag(id, tag) as unknown as Promise<void>,
  removeTag: (id: string, tag: string) => ModsService.RemoveTag(id, tag) as unknown as Promise<void>,
  trayItems: () => TrayService.Scan() as unknown as Promise<TrayItem[]>,

  packs: () => PackService.List() as unknown as Promise<Pack[]>,
  packRoot: () => PackService.Root() as unknown as Promise<string>,
  setPack: (code: string, enabled: boolean) =>
    PackService.SetEnabled(code, enabled) as unknown as Promise<void>,
  pickInstallFolder: () =>
    ConfigService.PickInstallFolder() as unknown as Promise<string>,
  openLink: (url: string) => ConfigService.OpenLink(url) as unknown as Promise<void>,

  /** Files dropped on a data-file-drop-target element. Returns an unsubscribe. */
  onFilesDropped: (cb: (paths: string[]) => void) =>
    Events.On('files-dropped', (event: unknown) => {
      const payload = (event as { data?: string[] }).data ?? (event as string[])
      if (Array.isArray(payload) && payload.length > 0) cb(payload)
    }),

  backups: () => BackupService.List() as unknown as Promise<Backup[]>,
  backupNow: () => BackupService.BackupNow() as unknown as Promise<Backup>,
  restore: (id: string) => BackupService.Restore(id) as unknown as Promise<void>,
  deleteBackup: (id: string) => BackupService.Delete(id) as unknown as Promise<void>,
  backupStorage: () => BackupService.Storage() as unknown as Promise<Storage>,
  emptyQuarantine: () => BackupService.EmptyQuarantine() as unknown as Promise<void>,
  revealBackups: () => BackupService.Reveal() as unknown as Promise<void>,
  quarantine: () => BackupService.Quarantine() as unknown as Promise<Quarantine[]>,
  restoreQuarantine: (id: string) =>
    BackupService.RestoreQuarantine(id) as unknown as Promise<void>,
  revealQuarantine: () => BackupService.RevealQuarantine() as unknown as Promise<void>,
  openQuarantine: () => BackupService.OpenQuarantineWindow() as unknown as Promise<void>,

  /** Fires when the set-aside list changes. */
  onQuarantine: (cb: () => void) => Events.On('quarantine', () => cb()),

  lastException: () => ExceptionService.Latest() as unknown as Promise<Exception>,
  openException: () => ExceptionService.Open() as unknown as Promise<void>,

  /** Subscribe to file-work progress. Returns an unsubscribe function. */
  onProgress: (cb: (p: Progress) => void) =>
    Events.On('progress', (event: unknown) => {
      const payload = (event as { data?: Progress }).data ?? (event as Progress)
      cb(payload)
    }),
}
