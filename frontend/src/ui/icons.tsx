import backupIcon from '../assets/icons/backups.png'
import ccIcon from '../assets/icons/cc.png'
import fixIcon from '../assets/icons/fix.png'
import installIcon from '../assets/icons/install.png'
import libraryIcon from '../assets/icons/library.png'
import markIcon from '../assets/icons/mark.png'
import modIcon from '../assets/icons/mod.png'
import viewGridIcon from '../assets/icons/view-grid.png'
import viewListIcon from '../assets/icons/view-list.png'

/**
 * In-game art from TheSimKid's Sims 4 Modder Icons pack.
 * Instance codes listed so the source stays traceable. The rail icons come from
 * ClientFullBuild8 so each nav item reads as its own object and hue; the rest
 * come from Aspirations.
 */
const art = {
  mark: markIcon, // Aspirations 0x3E51D46C9F69ED10, also the app icon
  library: libraryIcon, // ClientFullBuild8 0x7A944BD8A480DCEF, brown book
  install: installIcon, // ClientFullBuild8 0x4F5CCE6CCEE86498, blue gift box
  backups: backupIcon, // ClientFullBuild8 0x398410F50050971C, red chest
  fix: fixIcon, // ClientFullBuild8 0x30A8FC337647DF1E, yellow screwdriver
  mod: modIcon, // Aspirations 0x70F4F2B9947509F6
  cc: ccIcon, // Aspirations 0xC75AD63DD21C9DE3
  viewList: viewListIcon, // ClientFullBuild7 0xFBA9F57941AEC08F, lined paper stack
  viewGrid: viewGridIcon, // ClientFullBuild8 0x6E6131EA6B757618, tablet grid
} as const

export type ArtName = keyof typeof art

export function Art({ name, size = 24, className = '' }: { name: ArtName; size?: number; className?: string }) {
  return (
    <img
      src={art[name]}
      alt=""
      width={size}
      height={size}
      aria-hidden="true"
      className={`object-contain ${className}`}
      style={{ width: size, height: size }}
    />
  )
}

