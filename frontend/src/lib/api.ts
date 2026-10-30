import { Events } from '@wailsio/runtime'
import * as BackupService from '../../bindings/nooboo/services/backupservice'
import * as ConfigService from '../../bindings/nooboo/services/configservice'
import * as ExceptionService from '../../bindings/nooboo/services/exceptionservice'
import * as ModsService from '../../bindings/nooboo/services/modsservice'
import * as UpdateService from '../../bindings/nooboo/services/updateservice'
import type { AppInfo, Backup, Exception, GameFolder, Mod, Progress, Storage, UpdateInfo } from '../../bindings/nooboo/models/models'

export type { AppInfo, Backup, Exception, GameFolder, Mod, Progress, Storage, UpdateInfo }

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
  install: (req: InstallRequest) => ModsService.Install(req as never) as unknown as Promise<Mod>,
  deleteMod: (id: string) => ModsService.Delete(id) as unknown as Promise<void>,

  backups: () => BackupService.List() as unknown as Promise<Backup[]>,
  backupNow: () => BackupService.BackupNow() as unknown as Promise<Backup>,
  restore: (id: string) => BackupService.Restore(id) as unknown as Promise<void>,
  deleteBackup: (id: string) => BackupService.Delete(id) as unknown as Promise<void>,
  backupStorage: () => BackupService.Storage() as unknown as Promise<Storage>,
  emptyQuarantine: () => BackupService.EmptyQuarantine() as unknown as Promise<void>,
  revealBackups: () => BackupService.Reveal() as unknown as Promise<void>,

  lastException: () => ExceptionService.Latest() as unknown as Promise<Exception>,
  openException: () => ExceptionService.Open() as unknown as Promise<void>,

  /** Subscribe to file-work progress. Returns an unsubscribe function. */
  onProgress: (cb: (p: Progress) => void) =>
    Events.On('progress', (event: unknown) => {
      const payload = (event as { data?: Progress }).data ?? (event as Progress)
      cb(payload)
    }),
}
