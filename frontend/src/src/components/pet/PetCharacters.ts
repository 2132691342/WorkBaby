/**
 * 内置桌宠角色 SVG（阶段 4-1 · 内置桌宠角色 + 情绪系统）。
 *
 * <p>5 套默认可爱动漫风格宠物：小猫咪/小兔子/小狗狗/小熊猫/机器人。
 * 每套支持多种情绪（idle/happy/thinking/sleepy/excited），每种情绪有不同表情。
 *
 * <p>
 */

export type PetMood = 'idle' | 'happy' | 'thinking' | 'sleepy' | 'excited'

export interface PetCharacter {
  id: string
  nameKey: string
  descriptionKey: string
  palette: [string, string] // [主色, 辅助色]
}

export const PET_CHARACTERS: PetCharacter[] = [
  { id: 'kitten', nameKey: 'pet.kitten', descriptionKey: 'pet.kittenDesc', palette: ['#ffb7c9', '#ff8fab'] },
  { id: 'bunny', nameKey: 'pet.bunny', descriptionKey: 'pet.bunnyDesc', palette: ['#e8b04b', '#f0c060'] },
  { id: 'puppy', nameKey: 'pet.puppy', descriptionKey: 'pet.puppyDesc', palette: ['#9bd0f5', '#64b5f6'] },
  { id: 'panda', nameKey: 'pet.panda', descriptionKey: 'pet.pandaDesc', palette: ['#ffffff', '#4a3f45'] },
  { id: 'robot', nameKey: 'pet.robot', descriptionKey: 'pet.robotDesc', palette: ['#cfc4ff', '#a78bfa'] },
  { id: 'dragon', nameKey: 'pet.dragon', descriptionKey: 'pet.dragonDesc', palette: ['#81c784', '#4caf50'] },
  { id: 'unicorn', nameKey: 'pet.unicorn', descriptionKey: 'pet.unicornDesc', palette: ['#f8bbd0', '#e91e63'] },
  { id: 'slime', nameKey: 'pet.slime', descriptionKey: 'pet.slimeDesc', palette: ['#a5d6a7', '#66bb6a'] },
  { id: 'angel', nameKey: 'pet.angel', descriptionKey: 'pet.angelDesc', palette: ['#fff9c4', '#fff176'] },
  { id: 'devil', nameKey: 'pet.devil', descriptionKey: 'pet.devilDesc', palette: ['#ef9a9a', '#ef5350'] },
  { id: 'alien', nameKey: 'pet.alien', descriptionKey: 'pet.alienDesc', palette: ['#b39ddb', '#7e57c2'] },
  { id: 'phoenix', nameKey: 'pet.phoenix', descriptionKey: 'pet.phoenixDesc', palette: ['#ffcc80', '#ffa726'] },
  { id: 'mermaid', nameKey: 'pet.mermaid', descriptionKey: 'pet.mermaidDesc', palette: ['#80deea', '#26c6da'] },
  { id: 'witch', nameKey: 'pet.witch', descriptionKey: 'pet.witchDesc', palette: ['#ce93d8', '#ab47bc'] }
]

export const PET_MOODS: { id: PetMood; labelKey: string; icon: string }[] = [
  { id: 'idle', labelKey: 'pet.mood.idle', icon: '😊' },
  { id: 'happy', labelKey: 'pet.mood.happy', icon: '😄' },
  { id: 'thinking', labelKey: 'pet.mood.thinking', icon: '🤔' },
  { id: 'sleepy', labelKey: 'pet.mood.sleepy', icon: '😴' },
  { id: 'excited', labelKey: 'pet.mood.excited', icon: '🤩' }
]

/** 小猫咪 SVG */
export function kittenSvg(size: number = 100): string {
  return `<svg width="${size}" height="${size}" viewBox="0 0 100 100" fill="none" xmlns="http://www.w3.org/2000/svg">
    <!-- 身体 -->
    <ellipse cx="50" cy="65" rx="24" ry="20" fill="#ffb7c9"/>
    <!-- 头 -->
    <circle cx="50" cy="40" r="18" fill="#ffc8d6"/>
    <!-- 左耳 -->
    <polygon points="36,26 42,38 30,34" fill="#ffb7c9"/>
    <!-- 右耳 -->
    <polygon points="64,26 58,38 70,34" fill="#ffb7c9"/>
    <!-- 左耳内 -->
    <polygon points="37,29 41,36 33,33" fill="#ff8fab"/>
    <!-- 右耳内 -->
    <polygon points="63,29 59,36 67,33" fill="#ff8fab"/>
    <!-- 左眼 -->
    <ellipse cx="43" cy="39" rx="3" ry="4" fill="#4a3f45"/>
    <!-- 右眼 -->
    <ellipse cx="57" cy="39" rx="3" ry="4" fill="#4a3f45"/>
    <!-- 眼睛高光 -->
    <circle cx="44" cy="37" r="1" fill="white"/>
    <circle cx="58" cy="37" r="1" fill="white"/>
    <!-- 鼻子 -->
    <ellipse cx="50" cy="45" rx="2" ry="1.5" fill="#ff8fab"/>
    <!-- 嘴巴 -->
    <path d="M46,48 Q50,52 54,48" stroke="#4a3f45" stroke-width="1.5" fill="none" stroke-linecap="round"/>
    <!-- 胡须 -->
    <line x1="28" y1="43" x2="40" y2="45" stroke="#d4a5b0" stroke-width="1"/>
    <line x1="28" y1="47" x2="40" y2="47" stroke="#d4a5b0" stroke-width="1"/>
    <line x1="60" y1="45" x2="72" y2="43" stroke="#d4a5b0" stroke-width="1"/>
    <line x1="60" y1="47" x2="72" y2="47" stroke="#d4a5b0" stroke-width="1"/>
    <!-- 尾巴 -->
    <path d="M72,65 Q82,55 78,45" stroke="#ffb7c9" stroke-width="5" fill="none" stroke-linecap="round"/>
    <!-- 肚子 -->
    <ellipse cx="50" cy="68" rx="14" ry="10" fill="#ffe4ec"/>
  </svg>`
}

/** 小兔子 SVG */
export function bunnySvg(size: number = 100): string {
  return `<svg width="${size}" height="${size}" viewBox="0 0 100 100" fill="none" xmlns="http://www.w3.org/2000/svg">
    <!-- 身体 -->
    <ellipse cx="50" cy="68" rx="22" ry="18" fill="#e8b04b"/>
    <!-- 头 -->
    <circle cx="50" cy="42" r="16" fill="#f0c060"/>
    <!-- 左耳 -->
    <ellipse cx="43" cy="22" rx="5" ry="14" fill="#f0c060"/>
    <!-- 右耳 -->
    <ellipse cx="57" cy="22" rx="5" ry="14" fill="#f0c060"/>
    <!-- 左耳内 -->
    <ellipse cx="43" cy="22" rx="2.5" ry="8" fill="#e8a030"/>
    <!-- 右耳内 -->
    <ellipse cx="57" cy="22" rx="2.5" ry="8" fill="#e8a030"/>
    <!-- 左眼 -->
    <circle cx="44" cy="40" r="3" fill="#4a3f45"/>
    <!-- 右眼 -->
    <circle cx="56" cy="40" r="3" fill="#4a3f45"/>
    <!-- 眼睛高光 -->
    <circle cx="45" cy="39" r="1" fill="white"/>
    <circle cx="57" cy="39" r="1" fill="white"/>
    <!-- 鼻子 -->
    <ellipse cx="50" cy="46" rx="2" ry="1.5" fill="#e8a030"/>
    <!-- 嘴巴 -->
    <path d="M47,49 L50,52 L53,49" stroke="#4a3f45" stroke-width="1" fill="none" stroke-linecap="round" stroke-linejoin="round"/>
    <!-- 脸颊 -->
    <circle cx="38" cy="45" r="4" fill="#ffd4a8" opacity="0.6"/>
    <circle cx="62" cy="45" r="4" fill="#ffd4a8" opacity="0.6"/>
    <!-- 肚子 -->
    <ellipse cx="50" cy="70" rx="12" ry="9" fill="#f8d89a"/>
  </svg>`
}

/** 小狗狗 SVG */
export function puppySvg(size: number = 100): string {
  return `<svg width="${size}" height="${size}" viewBox="0 0 100 100" fill="none" xmlns="http://www.w3.org/2000/svg">
    <!-- 身体 -->
    <ellipse cx="50" cy="65" rx="24" ry="20" fill="#9bd0f5"/>
    <!-- 头 -->
    <circle cx="50" cy="38" r="18" fill="#b8dcf8"/>
    <!-- 左耳（下垂） -->
    <ellipse cx="34" cy="38" rx="7" ry="12" fill="#64b5f6" transform="rotate(-15 34 38)"/>
    <!-- 右耳（下垂） -->
    <ellipse cx="66" cy="38" rx="7" ry="12" fill="#64b5f6" transform="rotate(15 66 38)"/>
    <!-- 左眼 -->
    <circle cx="43" cy="36" r="3.5" fill="#4a3f45"/>
    <!-- 右眼 -->
    <circle cx="57" cy="36" r="3.5" fill="#4a3f45"/>
    <!-- 眼睛高光 -->
    <circle cx="44" cy="35" r="1.2" fill="white"/>
    <circle cx="58" cy="35" r="1.2" fill="white"/>
    <!-- 鼻子 -->
    <ellipse cx="50" cy="44" rx="3.5" ry="2.5" fill="#4a3f45"/>
    <!-- 嘴巴 -->
    <path d="M46,48 Q50,53 54,48" stroke="#4a3f45" stroke-width="1.5" fill="none" stroke-linecap="round"/>
    <!-- 舌头 -->
    <ellipse cx="50" cy="52" rx="3" ry="2" fill="#ff8fab"/>
    <!-- 斑点 -->
    <ellipse cx="60" cy="30" rx="5" ry="4" fill="#64b5f6" opacity="0.5"/>
    <!-- 肚子 -->
    <ellipse cx="50" cy="68" rx="14" ry="10" fill="#e8f4fd"/>
    <!-- 尾巴 -->
    <path d="M72,60 Q80,50 76,42" stroke="#9bd0f5" stroke-width="5" fill="none" stroke-linecap="round"/>
  </svg>`
}

/** 小熊猫 SVG */
export function pandaSvg(size: number = 100): string {
  return `<svg width="${size}" height="${size}" viewBox="0 0 100 100" fill="none" xmlns="http://www.w3.org/2000/svg">
    <!-- 身体 -->
    <ellipse cx="50" cy="68" rx="24" ry="18" fill="#4a3f45"/>
    <!-- 头 -->
    <circle cx="50" cy="40" r="20" fill="white"/>
    <!-- 左耳 -->
    <circle cx="34" cy="26" r="7" fill="#4a3f45"/>
    <!-- 右耳 -->
    <circle cx="66" cy="26" r="7" fill="#4a3f45"/>
    <!-- 左眼圈 -->
    <ellipse cx="42" cy="40" rx="6" ry="7" fill="#4a3f45"/>
    <!-- 右眼圈 -->
    <ellipse cx="58" cy="40" rx="6" ry="7" fill="#4a3f45"/>
    <!-- 左眼 -->
    <circle cx="42" cy="40" r="3" fill="white"/>
    <!-- 右眼 -->
    <circle cx="58" cy="40" r="3" fill="white"/>
    <!-- 左眼珠 -->
    <circle cx="43" cy="39" r="1.5" fill="#4a3f45"/>
    <!-- 右眼珠 -->
    <circle cx="59" cy="39" r="1.5" fill="#4a3f45"/>
    <!-- 鼻子 -->
    <ellipse cx="50" cy="48" rx="3" ry="2" fill="#4a3f45"/>
    <!-- 嘴巴 -->
    <path d="M47,51 Q50,54 53,51" stroke="#4a3f45" stroke-width="1.5" fill="none" stroke-linecap="round"/>
    <!-- 肚子 -->
    <ellipse cx="50" cy="70" rx="14" ry="10" fill="white"/>
    <!-- 腮红 -->
    <circle cx="36" cy="46" r="3" fill="#ffb7c9" opacity="0.5"/>
    <circle cx="64" cy="46" r="3" fill="#ffb7c9" opacity="0.5"/>
  </svg>`
}

/** 机器人 SVG */
export function robotSvg(size: number = 100): string {
  return `<svg width="${size}" height="${size}" viewBox="0 0 100 100" fill="none" xmlns="http://www.w3.org/2000/svg">
    <!-- 身体 -->
    <rect x="32" y="50" width="36" height="32" rx="6" fill="#cfc4ff"/>
    <!-- 头 -->
    <rect x="30" y="22" width="40" height="30" rx="8" fill="#ddd6fe"/>
    <!-- 天线 -->
    <line x1="50" y1="22" x2="50" y2="12" stroke="#a78bfa" stroke-width="3"/>
    <circle cx="50" cy="10" r="4" fill="#a78bfa"/>
    <!-- 左眼 -->
    <circle cx="40" cy="35" r="5" fill="white"/>
    <!-- 右眼 -->
    <circle cx="60" cy="35" r="5" fill="white"/>
    <!-- 左眼珠 -->
    <circle cx="42" cy="35" r="2.5" fill="#4a3f45"/>
    <!-- 右眼珠 -->
    <circle cx="62" cy="35" r="2.5" fill="#4a3f45"/>
    <!-- 嘴巴 -->
    <rect x="40" y="44" width="20" height="4" rx="2" fill="#a78bfa"/>
    <!-- 身体装饰 -->
    <rect x="38" y="56" width="24" height="3" rx="1.5" fill="#a78bfa"/>
    <rect x="38" y="62" width="24" height="3" rx="1.5" fill="#a78bfa"/>
    <rect x="38" y="68" width="24" height="3" rx="1.5" fill="#a78bfa"/>
    <!-- 左臂 -->
    <rect x="22" y="54" width="8" height="20" rx="4" fill="#cfc4ff"/>
    <!-- 右臂 -->
    <rect x="70" y="54" width="8" height="20" rx="4" fill="#cfc4ff"/>
    <!-- 左腿 -->
    <rect x="38" y="82" width="8" height="12" rx="4" fill="#a78bfa"/>
    <!-- 右腿 -->
    <rect x="54" y="82" width="8" height="12" rx="4" fill="#a78bfa"/>
    <!-- 心 -->
    <circle cx="50" cy="75" r="4" fill="#ff8fab"/>
  </svg>`
}

/** 简化版通用SVG生成函数（为新增角色提供默认样式） */
export function generateDefaultPetSvg(characterID: string, size: number = 100): string {
  const character = PET_CHARACTERS.find(c => c.id === characterID)
  const [primary, secondary] = character?.palette || ['#ffb7c9', '#ff8fab']
  return `<svg width="${size}" height="${size}" viewBox="0 0 100 100" fill="none" xmlns="http://www.w3.org/2000/svg">
    <!-- 身体 -->
    <ellipse cx="50" cy="65" rx="24" ry="20" fill="${primary}"/>
    <!-- 头 -->
    <circle cx="50" cy="40" r="18" fill="${primary}"/>
    <!-- 左耳 -->
    <polygon points="36,26 42,38 30,34" fill="${secondary}"/>
    <!-- 右耳 -->
    <polygon points="64,26 58,38 70,34" fill="${secondary}"/>
    <!-- 左眼 -->
    <ellipse cx="43" cy="39" rx="3" ry="4" fill="#4a3f45"/>
    <!-- 右眼 -->
    <ellipse cx="57" cy="39" rx="3" ry="4" fill="#4a3f45"/>
    <!-- 眼睛高光 -->
    <circle cx="44" cy="37" r="1" fill="white"/>
    <circle cx="58" cy="37" r="1" fill="white"/>
    <!-- 鼻子 -->
    <ellipse cx="50" cy="45" rx="2" ry="1.5" fill="${secondary}"/>
    <!-- 嘴巴 -->
    <path d="M46,48 Q50,52 54,48" stroke="#4a3f45" stroke-width="1.5" fill="none" stroke-linecap="round"/>
  </svg>`
}

/** 根据角色 ID 获取 SVG */
export function getPetSvg(characterID: string, size: number = 100): string {
  switch (characterID) {
    case 'kitten': return kittenSvg(size)
    case 'bunny': return bunnySvg(size)
    case 'puppy': return puppySvg(size)
    case 'panda': return pandaSvg(size)
    case 'robot': return robotSvg(size)
    default: return generateDefaultPetSvg(characterID, size)
  }
}

/** 获取情绪对应的 SVG 样式覆盖（通过 CSS filter 实现情绪变化） */
export function getMoodFilter(mood: PetMood): string {
  switch (mood) {
    case 'happy':
      return 'brightness(1.05) saturate(1.1)'
    case 'thinking':
      return 'brightness(0.98) contrast(1.02)'
    case 'sleepy':
      return 'brightness(0.92) saturate(0.8)'
    case 'excited':
      return 'brightness(1.08) saturate(1.2) hue-rotate(-5deg)'
    default:
      return 'none'
  }
}
