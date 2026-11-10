"use strict";
// Мультиязычность интерфейса: RU (исходный), EN, UK.
// Словарь: русская строка -> [английский, украинский].

const I18N_LANGS = { ru: "Русский", en: "English", uk: "Українська" };

const I18N = {
  "Или папка миссии": ["Or a mission folder", "Або папка місії"],
  "Максимум 200 файлов за одну проверку": ["Maximum 200 files per validation", "Максимум 200 файлів за одну перевірку"],
  "Слишком большой объём файлов для одной проверки": ["Files exceed the size limit for one validation", "Завеликий обсяг файлів для однієї перевірки"],
  "Проверка экономики DayZ 1.30: nominal/min, флаги, шансы групп 0..1 и относительные веса предметов. При совместной загрузке проверяются дубли между файлами, ссылки на пресеты и подключения cfgeconomycore.xml; свои категории и тиры читаются из файлов limitsdefinition.": ["DayZ 1.30 economy checks: nominal/min, flags, group probabilities 0..1 and relative item weights. Combined uploads also check duplicates across files, preset references and cfgeconomycore.xml registrations; custom categories and tiers are read from limitsdefinition files.", "Перевірка економіки DayZ 1.30: nominal/min, прапорці, шанси груп 0..1 та відносні ваги предметів. Спільне завантаження перевіряє дублікати між файлами, посилання на пресети та підключення cfgeconomycore.xml; власні категорії й тіри читаються з файлів limitsdefinition."],
  "Из папки миссии читаются только XML и JSON, без storage. Максимум 200 файлов, 128 МБ на файл и 256 МБ суммарно. Файлы не изменяются. Красным — ошибки, жёлтым — замечания; для неполной загрузки некоторые связи проверить нельзя.": ["Only XML and JSON are read from a mission folder, excluding storage. Maximum 200 files, 128 MB per file and 256 MB per upload. Files are not modified. Red means errors, yellow means warnings; some links cannot be checked with an incomplete upload.", "Із папки місії читаються лише XML та JSON, без storage. Максимум 200 файлів, 128 МБ на файл і 256 МБ загалом. Файли не змінюються. Червоним — помилки, жовтим — зауваження; за неповного завантаження деякі зв'язки перевірити неможливо."],
  "Создание рекваер-аддонов (override) для модов DayZ": ["Building require-addons (overrides) for DayZ mods", "Створення рекваєр-аддонів (override) для модів DayZ"],
  "Мод": ["Mod", "Мод"],
  "Предметы": ["Items", "Предмети"],
  "Генерация аддона + настройки DayZ Tools (подпись, ключи, бинаризация)": ["Addon generation + DayZ Tools settings (signing, keys, binarization)", "Генерація аддона + налаштування DayZ Tools (підпис, ключі, бінаризація)"],
  "Генерация": ["Generate", "Генерація"],
  "Сверка старой и новой версии мода": ["Compare old and new mod versions", "Порівняння старої та нової версій мода"],
  "Сверка версий": ["Version diff", "Порівняння версій"],
  "Миграция и оптимизация types.xml": ["types.xml migration and optimization", "Міграція та оптимізація types.xml"],
  "Слияние нескольких оверрайдов в один": ["Merge several overrides into one", "Злиття кількох оверрайдів в один"],
  "Слияние": ["Merge", "Злиття"],
  "DayZ Expansion: проверка и слияние квестов": ["DayZ Expansion: quest validation and merging", "DayZ Expansion: перевірка та злиття квестів"],
  "Exp. Квесты": ["Exp. Quests", "Exp. Квести"],
  "DayZ Expansion: проверка и слияние лодаутов": ["DayZ Expansion: loadout validation and merging", "DayZ Expansion: перевірка та злиття лодаутів"],
  "Exp. Лодауты": ["Exp. Loadouts", "Exp. Лодаути"],
  "Проверка файлов JSON/XML с указанием строк и подсказками": ["Validate JSON/XML files with line numbers and hints", "Перевірка файлів JSON/XML із номерами рядків і підказками"],
  "Проверка файлов": ["File check", "Перевірка файлів"],
  "Все действия и ошибки программы": ["All program actions and errors", "Усі дії та помилки програми"],
  "Журнал": ["Log", "Журнал"],
  "Настроить список языков": ["Configure language list", "Налаштувати список мов"],

  "Выберите PBO мода для анализа": ["Select mod PBO for analysis", "Оберіть PBO мода для аналізу"],
  "Перетащите сюда один или несколько": ["Drag & drop one or more", "Перетягніть сюди один або кілька"],
  "или": ["or", "або"],
  "Выбрать файлы…": ["Choose files…", "Обрати файли…"],
  "или укажите путь на этом компьютере": ["or enter a path on this computer", "або вкажіть шлях на цьому комп'ютері"],
  "Анализировать путь": ["Analyze path", "Аналізувати шлях"],
  "Можно указать папку мода (например @DukesGhillies) — программа найдёт все .pbo внутри. Файл config.bin будет распакован и дебинаризован автоматически.": ["You can point to a mod folder (e.g. @DukesGhillies) — the program will find all .pbo inside. config.bin is unpacked and de-binarized automatically.", "Можна вказати теку мода (наприклад @DukesGhillies) — програма знайде всі .pbo всередині. Файл config.bin буде розпаковано та дебінаризовано автоматично."],
  "Для больших модов (сотни МБ и больше) указывайте путь": ["For big mods (hundreds of MB and more) use the path field", "Для великих модів (сотні МБ і більше) вказуйте шлях"],
  "— это мгновенно: без копирования файлов. Перетаскивание файлов сначала передаёт их во временную папку программы, для ган-пака на несколько ГБ это занимает минуты.": ["— it is instant: no file copying. Drag & drop first uploads files into a temp folder; for a multi-GB gun pack that takes minutes.", "— це миттєво: без копіювання файлів. Перетягування спочатку передає файли до тимчасової теки; для ган-пака на кілька ГБ це триває хвилини."],
  "Настройки DayZ Tools (подпись, ключи, бинаризация) — на вкладке «3 Генерация», доступны в любой момент.": ["DayZ Tools settings (signing, keys, binarization) live on the \"3 Generate\" tab and are available at any time.", "Налаштування DayZ Tools (підпис, ключі, бінаризація) — на вкладці «3 Генерація», доступні будь-коли."],
  "Результат анализа": ["Analysis result", "Результат аналізу"],
  "Перейти к предметам →": ["Go to items →", "Перейти до предметів →"],
  "Анализирую PBO…": ["Analyzing PBO…", "Аналізую PBO…"],
  "Журнал анализа": ["Analysis log", "Журнал аналізу"],

  "Поиск по класснейму…": ["Search by classname…", "Пошук за класснеймом…"],
  "Все категории": ["All categories", "Усі категорії"],
  "только scope=2": ["scope=2 only", "лише scope=2"],
  "только с правками": ["edited only", "лише з правками"],
  "пустые колонки": ["empty columns", "порожні колонки"],
  "Отметить видимые": ["Select visible", "Позначити видимі"],
  "Снять все": ["Unselect all", "Зняти всі"],
  "Массовая правка…": ["Bulk edit…", "Масова правка…"],
  "Кликните по ячейке, чтобы изменить значение. Кнопка «+» — добавить параметр, которого у предмета не было (например карго-слоты). Отмеченные предметы попадут в аддон. Серым курсивом — значения, унаследованные от родительских классов; «—» — параметр не найден в моде (унаследован от ванильного класса игры).": ["Click a cell to change a value. The \"+\" button adds a parameter the item did not have (e.g. cargo slots). Checked items go into the addon. Grey italics = values inherited from parent classes; \"—\" = parameter not found in the mod (inherited from a vanilla game class).", "Клікніть по клітинці, щоб змінити значення. Кнопка «+» — додати параметр, якого предмет не мав (наприклад карго-слоти). Позначені предмети потраплять в аддон. Сірим курсивом — значення, успадковані від батьківських класів; «—» — параметр не знайдено в моді (успадковано від ванільного класу гри)."],
  "Нет данных — сначала проанализируйте мод на вкладке «1 Мод».": ["No data — analyze a mod on the \"1 Mod\" tab first.", "Немає даних — спочатку проаналізуйте мод на вкладці «1 Мод»."],
  "К генерации аддона →": ["To addon generation →", "До генерації аддона →"],

  "DayZ Tools — подпись и сборка (необязательно)": ["DayZ Tools — signing and building (optional)", "DayZ Tools — підпис і збірка (необов'язково)"],
  "Автопоиск": ["Auto-detect", "Автопошук"],
  "Проверить": ["Check", "Перевірити"],
  "Подписать PBO автоматически (DSSignFile)": ["Sign PBO automatically (DSSignFile)", "Підписати PBO автоматично (DSSignFile)"],
  "Приватный ключ (.biprivatekey)": ["Private key (.biprivatekey)", "Приватний ключ (.biprivatekey)"],
  "Нет ключа? Создать новую пару ключей (DSCreateKey)": ["No key? Create a new key pair (DSCreateKey)", "Немає ключа? Створити нову пару ключів (DSCreateKey)"],
  "Имя, напр. MyServerKey": ["Name, e.g. MyServerKey", "Ім'я, напр. MyServerKey"],
  "Папка для ключей, напр. C:\\DayZKeys": ["Folder for keys, e.g. C:\\DayZKeys", "Тека для ключів, напр. C:\\DayZKeys"],
  "Создать": ["Create", "Створити"],
  "Появятся два файла: .biprivatekey (секретный, для подписи) и .bikey (публичный, в keys сервера). Путь подставится автоматически.": ["Two files will appear: .biprivatekey (secret, for signing) and .bikey (public, for the server keys folder). The path is filled in automatically.", "З'являться два файли: .biprivatekey (секретний, для підпису) та .bikey (публічний, до keys сервера). Шлях підставиться автоматично."],
  "Бинаризовать config.cpp → config.bin (CfgConvert)": ["Binarize config.cpp → config.bin (CfgConvert)", "Бінаризувати config.cpp → config.bin (CfgConvert)"],
  "Упаковать официальным FileBank вместо встроенного упаковщика": ["Pack with official FileBank instead of the built-in packer", "Пакувати офіційним FileBank замість вбудованого пакувальника"],
  "Программа не копирует инструменты — вызывает их прямо из указанной папки при каждой сборке, поэтому обновления DayZ Tools из Steam подхватываются автоматически. Путь и настройки сохраняются в settings.json.": ["The program does not copy the tools — it runs them straight from the given folder on every build, so Steam updates of DayZ Tools are picked up automatically. Path and settings are stored in settings.json.", "Програма не копіює інструменти — викликає їх прямо із вказаної теки під час кожної збірки, тож оновлення DayZ Tools зі Steam підхоплюються автоматично. Шлях і налаштування зберігаються в settings.json."],

  "Настройки аддона": ["Addon settings", "Налаштування аддона"],
  "Имя аддона (классы CfgPatches, имя PBO)": ["Addon name (CfgPatches class, PBO name)", "Ім'я аддона (класи CfgPatches, ім'я PBO)"],
  "requiredAddons — аддоны исходного мода (через запятую)": ["requiredAddons — source mod addons (comma-separated)", "requiredAddons — аддони вихідного мода (через кому)"],
  "Собрать готовый .pbo (останется только подписать)": ["Build ready .pbo (only signing remains)", "Зібрати готовий .pbo (залишиться лише підписати)"],
  "Предпросмотр config.cpp": ["Preview config.cpp", "Попередній перегляд config.cpp"],
  "⬇ Скачать ZIP": ["⬇ Download ZIP", "⬇ Завантажити ZIP"],
  "или сохранить в папку на этом компьютере": ["or save to a folder on this computer", "або зберегти до теки на цьому комп'ютері"],
  "Сохранить": ["Save", "Зберегти"],
  "После генерации: подпишите PBO своим ключом (или включите автоподпись выше), положите папку @Аддон на сервер и клиентам, ключ .bikey — в keys. Подробная инструкция — в README_RU.txt внутри архива.": ["After generation: sign the PBO with your key (or enable auto-sign above), put the @Addon folder on the server and clients, and the .bikey into keys. Detailed instructions are in README_RU.txt inside the archive.", "Після генерації: підпишіть PBO своїм ключем (або ввімкніть автопідпис вище), покладіть теку @Аддон на сервер і клієнтам, ключ .bikey — до keys. Докладна інструкція — у README_RU.txt всередині архіву."],
  "Нажмите «Предпросмотр config.cpp», чтобы увидеть будущий конфиг аддона.": ["Press \"Preview config.cpp\" to see the future addon config.", "Натисніть «Попередній перегляд config.cpp», щоб побачити майбутній конфіг аддона."],

  "Сверка версий мода — что изменилось после обновления": ["Mod version diff — what changed after an update", "Порівняння версій мода — що змінилось після оновлення"],
  "Загрузите PBO старой версии и PBO новой версии (можно несколько с каждой стороны, либо укажите пути к папкам). Программа сравнит все класснеймы и покажет: новые предметы, удалённые, и по каждому изменённому классу — какие именно параметры автор поменял (базовые классы, ТТХ, описания).": ["Upload old-version and new-version PBOs (several per side allowed, or specify folder paths). The program compares all classnames and shows: new items, removed ones, and for each changed class — exactly which parameters the author changed (base classes, stats, descriptions).", "Завантажте PBO старої та нової версій (можна кілька з кожного боку, або вкажіть шляхи до тек). Програма порівняє всі класснейми та покаже: нові предмети, видалені, і для кожного зміненого класу — які саме параметри змінив автор (базові класи, ТТХ, описи)."],
  "СТАРАЯ версия — .pbo файлы": ["OLD version — .pbo files", "СТАРА версія — .pbo файли"],
  "…или путь к файлу/папке старой версии": ["…or path to old version file/folder", "…або шлях до файлу/теки старої версії"],
  "НОВАЯ версия — .pbo файлы": ["NEW version — .pbo files", "НОВА версія — .pbo файли"],
  "…или путь к файлу/папке новой версии": ["…or path to new version file/folder", "…або шлях до файлу/теки нової версії"],
  "Сравнить версии": ["Compare versions", "Порівняти версії"],
  "⬇ Скачать отчёт (.txt)": ["⬇ Download report (.txt)", "⬇ Завантажити звіт (.txt)"],
  "Результат сверки": ["Diff result", "Результат порівняння"],

  "Миграция types.xml — после вайпа или обновления": ["types.xml migration — after a wipe or update", "Міграція types.xml — після вайпу або оновлення"],
  "Ваши сохранённые настройки лута переносятся в новый (ванильный) types.xml: совпадающие записи берутся из ВАШЕГО файла, новые класснеймы остаются с ванильными значениями и попадают в отчёт.": ["Your saved loot settings are transferred into the new (vanilla) types.xml: matching entries come from YOUR file, new classnames keep vanilla values and are listed in the report.", "Ваші збережені налаштування луту переносяться в новий (ванільний) types.xml: збіжні записи беруться з ВАШОГО файлу, нові класснейми лишаються з ванільними значеннями та потрапляють до звіту."],
  "МОЙ сохранённый types.xml (до вайпа)": ["MY saved types.xml (before wipe)", "МІЙ збережений types.xml (до вайпу)"],
  "НОВЫЙ types.xml (после вайпа/обновления)": ["NEW types.xml (after wipe/update)", "НОВИЙ types.xml (після вайпу/оновлення)"],
  "Сохранять мои записи, которых нет в новом файле (предметы модов)": ["Keep my entries missing from the new file (mod items)", "Зберігати мої записи, яких немає в новому файлі (предмети модів)"],
  "Объединить": ["Merge", "Об'єднати"],
  "⬇ Скачать объединённый types.xml": ["⬇ Download merged types.xml", "⬇ Завантажити об'єднаний types.xml"],
  "⬇ Отчёт (.txt)": ["⬇ Report (.txt)", "⬇ Звіт (.txt)"],
  "Проверка и оптимизация types.xml": ["types.xml check and optimization", "Перевірка та оптимізація types.xml"],
  "Проверка валидности и типовых ошибок: nominal меньше min, несогласованные quantmin/quantmax, дубликаты, отсутствующие lifetime/flags, мусорные значения. Неспавнящиеся записи (nominal=0 и min=0) приводятся к короткому виду — только lifetime и flags. На выходе — чистый файл с единым форматированием.": ["Validity and common-error checks: nominal below min, inconsistent quantmin/quantmax, duplicates, missing lifetime/flags, garbage values. Non-spawning entries (nominal=0 and min=0) are reduced to the short form — lifetime and flags only. Output is a clean, uniformly formatted file.", "Перевірка валідності та типових помилок: nominal менший за min, неузгоджені quantmin/quantmax, дублікати, відсутні lifetime/flags, сміттєві значення. Неспавнні записи (nominal=0 та min=0) зводяться до короткого вигляду — лише lifetime і flags. На виході — чистий файл з єдиним форматуванням."],
  "types.xml для проверки": ["types.xml to check", "types.xml для перевірки"],
  "Упрощать неспавнящиеся записи (nominal=0 и min=0)": ["Simplify non-spawning entries (nominal=0 and min=0)", "Спрощувати неспавнні записи (nominal=0 та min=0)"],
  "Проверить и исправить": ["Check and fix", "Перевірити та виправити"],
  "⬇ Скачать исправленный types.xml": ["⬇ Download fixed types.xml", "⬇ Завантажити виправлений types.xml"],

  "Слияние оверрайд-аддонов — несколько в один": ["Merging override addons — several into one", "Злиття оверрайд-аддонів — кілька в один"],
  "Выберите 2 и более оверрайда (.pbo или config.cpp). Программа объединит их в один аддон: CfgPatches и requiredAddons собираются автоматически, одинаковые классы сливаются. При конфликте значений побеждает файл, выбранный ПОЗЖЕ (порядок в списке = приоритет), а каждый конфликт попадает в отчёт.": ["Select 2 or more overrides (.pbo or config.cpp). The program merges them into one addon: CfgPatches and requiredAddons are gathered automatically, identical classes are merged. On value conflicts the file selected LATER wins (list order = priority) and every conflict is reported.", "Оберіть 2 і більше оверрайдів (.pbo або config.cpp). Програма об'єднає їх в один аддон: CfgPatches і requiredAddons збираються автоматично, однакові класи зливаються. За конфлікту значень перемагає файл, обраний ПІЗНІШЕ (порядок у списку = пріоритет), а кожен конфлікт потрапляє до звіту."],
  "Файлы оверрайдов (.pbo / config.cpp) — порядок выбора важен": ["Override files (.pbo / config.cpp) — selection order matters", "Файли оверрайдів (.pbo / config.cpp) — порядок вибору важливий"],
  "Имя объединённого аддона": ["Merged addon name", "Ім'я об'єднаного аддона"],
  "Собрать готовый .pbo": ["Build ready .pbo", "Зібрати готовий .pbo"],
  "Подписать (ключ и DayZ Tools — из вкладки «Генерация»)": ["Sign (key and DayZ Tools from the \"Generate\" tab)", "Підписати (ключ і DayZ Tools — із вкладки «Генерація»)"],
  "Бинаризовать config (CfgConvert)": ["Binarize config (CfgConvert)", "Бінаризувати config (CfgConvert)"],
  "Слить оверрайды": ["Merge overrides", "Злити оверрайди"],
  "или сохранить в папку": ["or save to a folder", "або зберегти до теки"],
  "Слить и сохранить": ["Merge and save", "Злити та зберегти"],
  "Результат слияния": ["Merge result", "Результат злиття"],
  "Показать объединённый config.cpp": ["Show merged config.cpp", "Показати об'єднаний config.cpp"],

  "Expansion Квесты — проверка пака": ["Expansion Quests — pack check", "Expansion Квести — перевірка пака"],
  "Прикрепите ZIP вашей папки Quests (из профиля сервера) или укажите путь. Программа проверит все связи: квест → объективы, квест → NPC, цепочки FollowUp/PreQuest, дубликаты ID, битые JSON — и покажет квестовые линии целиком.": ["Attach a ZIP of your Quests folder (from the server profile) or enter a path. The program checks every link: quest → objectives, quest → NPCs, FollowUp/PreQuest chains, duplicate IDs, broken JSON — and shows whole quest lines.", "Прикріпіть ZIP вашої теки Quests (із профілю сервера) або вкажіть шлях. Програма перевірить усі зв'язки: квест → об'єктиви, квест → NPC, ланцюжки FollowUp/PreQuest, дублікати ID, биті JSON — і покаже квестові лінії цілком."],
  "Дополнительные проверки (лодауты NPC, класснеймы по types.xml)": ["Extra checks (NPC loadouts, classnames vs types.xml)", "Додаткові перевірки (лодаути NPC, класснейми за types.xml)"],
  "ZIP папки Loadouts (проверка лодаутов NPC)": ["Loadouts folder ZIP (NPC loadout check)", "ZIP теки Loadouts (перевірка лодаутів NPC)"],
  "types.xml (проверка класснеймов наград/коллекций)": ["types.xml (check reward/collection classnames)", "types.xml (перевірка класснеймів нагород/колекцій)"],
  "Проверить квесты": ["Check quests", "Перевірити квести"],
  "Слияние двух паков квестов (ваш + новый)": ["Merging two quest packs (yours + new)", "Злиття двох паків квестів (ваш + новий)"],
  "Конфликтующие ID квестов/объективов/NPC из нового пака будут автоматически перенумерованы, все ссылки — переписаны, идентичные файлы — пропущены. Прогресс игроков (PlayerData/GroupData) берётся только из ВАШЕГО пака.": ["Conflicting quest/objective/NPC IDs from the new pack are renumbered automatically, all references rewritten, identical files skipped. Player progress (PlayerData/GroupData) is taken from YOUR pack only.", "Конфліктні ID квестів/об'єктивів/NPC з нового пака буде автоматично перенумеровано, всі посилання — переписано, ідентичні файли — пропущено. Прогрес гравців (PlayerData/GroupData) береться лише з ВАШОГО пака."],
  "ВАШ пак (ZIP)": ["YOUR pack (ZIP)", "ВАШ пак (ZIP)"],
  "…или путь": ["…or path", "…або шлях"],
  "НОВЫЙ пак (ZIP)": ["NEW pack (ZIP)", "НОВИЙ пак (ZIP)"],
  "Слить паки (скачать ZIP)": ["Merge packs (download ZIP)", "Злити паки (завантажити ZIP)"],

  "Expansion Лодауты — проверка": ["Expansion Loadouts — check", "Expansion Лодаути — перевірка"],
  "Прикрепите ZIP папки Loadouts или укажите путь. Проверка: битый JSON, Chance вне 0..1, Quantity/Health где Min больше Max, пустые ClassName, список всех класснеймов (опционально — сверка с types.xml).": ["Attach a Loadouts folder ZIP or enter a path. Checks: broken JSON, Chance outside 0..1, Quantity/Health with Min above Max, empty ClassName, full classname list (optionally checked against types.xml).", "Прикріпіть ZIP теки Loadouts або вкажіть шлях. Перевірка: битий JSON, Chance поза 0..1, Quantity/Health де Min більший за Max, порожні ClassName, список усіх класснеймів (опційно — звірка з types.xml)."],
  "Проверить лодауты": ["Check loadouts", "Перевірити лодаути"],
  "Слияние двух папок лодаутов (ваши + новые)": ["Merging two loadout folders", "Злиття двох тек лодаутів"],
  "Одинаковые файлы пропускаются, при различии новый файл сохраняется с суффиксом _v2 (в отчёте будет напоминание обновить NPCLoadoutFile).": ["Identical files are skipped; on differences the new file is saved with the _v2 suffix (the report reminds you to update NPCLoadoutFile).", "Однакові файли пропускаються, за відмінності новий файл зберігається з суфіксом _v2 (у звіті буде нагадування оновити NPCLoadoutFile)."],
  "ВАШИ лодауты (ZIP)": ["YOUR loadouts (ZIP)", "ВАШІ лодаути (ZIP)"],
  "НОВЫЕ лодауты (ZIP)": ["NEW loadouts (ZIP)", "НОВІ лодаути (ZIP)"],
  "Слить (скачать ZIP)": ["Merge (download ZIP)", "Злити (завантажити ZIP)"],

  "Проверка файлов — JSON, XML, конфиги": ["File check — JSON, XML, configs", "Перевірка файлів — JSON, XML, конфіги"],
  "Прикрепите любые файлы сервера: types.xml, cfgspawnabletypes.xml, квесты/лодауты (.json), config.cpp, .layout и т.д. Программа найдёт ошибки с точным номером строки, покажет фрагмент файла и подскажет, как исправить.": ["Attach any server files: types.xml, cfgspawnabletypes.xml, quests/loadouts (.json), config.cpp, .layout etc. The program finds errors with exact line numbers, shows the file fragment and suggests a fix.", "Прикріпіть будь-які файли сервера: types.xml, cfgspawnabletypes.xml, квести/лодаути (.json), config.cpp, .layout тощо. Програма знайде помилки з точним номером рядка, покаже фрагмент файлу та підкаже, як виправити."],
  "Файлы для проверки": ["Files to check", "Файли для перевірки"],
  "Проверить файлы": ["Check files", "Перевірити файли"],
  "Формат файлов": ["File format", "Формат файлів"],
  "Определить автоматически": ["Auto-detect", "Визначити автоматично"],
  "types.xml (экономика)": ["types.xml (economy)", "types.xml (економіка)"],
  "Другой XML": ["Other XML", "Інший XML"],
  "JSON — квест/лодаут Expansion": ["JSON — Expansion quest/loadout", "JSON — квест/лодаут Expansion"],
  "config.cpp / .layout / текст": ["config.cpp / .layout / text", "config.cpp / .layout / текст"],
  "Обычно достаточно «Определить автоматически». Выберите конкретный формат, если у файла нет расширения или нужно убедиться, что это именно тот файл — например, что выбранный XML является настоящим types.xml.": ["Usually \"Auto-detect\" is enough. Pick a specific format if the file has no extension or you want to make sure it really is that file — for example, that the selected XML is a genuine types.xml.", "Зазвичай достатньо «Визначити автоматично». Оберіть конкретний формат, якщо файл не має розширення або треба переконатися, що це саме той файл — наприклад, що обраний XML є справжнім types.xml."],
  "Для types.xml, cfgspawnabletypes.xml и cfgrandompresets.xml выполняется проверка по правилам экономики DayZ (nominal/min, флаги 0/1, chance 0.0–1.0, тиры и юзейджи, дубликаты). Красным — ошибки (файл сломан), жёлтым — замечания (файл грузится, но стоит проверить).": ["For types.xml, cfgspawnabletypes.xml and cfgrandompresets.xml the file is checked against DayZ economy rules (nominal/min, flags 0/1, chance 0.0–1.0, tiers and usages, duplicates). Red = errors (file is broken), yellow = notices (file loads but is worth reviewing).", "Для types.xml, cfgspawnabletypes.xml і cfgrandompresets.xml виконується перевірка за правилами економіки DayZ (nominal/min, прапорці 0/1, chance 0.0–1.0, тіри та юзейджі, дублікати). Червоним — помилки (файл зламано), жовтим — зауваження (файл вантажиться, але варто перевірити)."],
  "Как исправить:": ["How to fix:", "Як виправити:"],
  "Журнал действий и ошибок": ["Action and error log", "Журнал дій та помилок"],
  "Скопировать журнал": ["Copy log", "Скопіювати журнал"],
  "Очистить": ["Clear", "Очистити"],
  "Здесь видно всё, что делает программа: анализ каждого PBO (включая зашифрованные и PBO с ресурсами), генерация аддона, вызовы DayZ Tools и все ошибки с указанием шага. Если что-то пошло не так — нажмите «Скопировать журнал» и пришлите текст разработчику.": ["Everything the program does is visible here: analysis of every PBO (including encrypted and resource ones), addon generation, DayZ Tools calls and all errors with the exact step. If something went wrong — press \"Copy log\" and send the text to the developer.", "Тут видно все, що робить програма: аналіз кожного PBO (включно з зашифрованими та ресурсними), генерація аддона, виклики DayZ Tools і всі помилки із зазначенням кроку. Якщо щось пішло не так — натисніть «Скопіювати журнал» і надішліть текст розробнику."],

  "Отмена": ["Cancel", "Скасувати"],
  "Сбросить правку": ["Reset edit", "Скинути правку"],
  "Добавить параметр": ["Add parameter", "Додати параметр"],
  "Параметр": ["Parameter", "Параметр"],
  "Путь в конфиге (классы через /, последний — свойство)": ["Config path (classes via /, last segment is the property)", "Шлях у конфізі (класи через /, останній — властивість)"],
  "Тип значения": ["Value type", "Тип значення"],
  "авто (число / {массив} / строка)": ["auto (number / {array} / string)", "авто (число / {масив} / рядок)"],
  "число": ["number", "число"],
  "целое": ["integer", "ціле"],
  "строка (текст)": ["string", "рядок"],
  "пара чисел Ш,В": ["number pair W,H", "пара чисел Ш,В"],
  "список строк через запятую": ["comma-separated string list", "список рядків через кому"],
  "Значение": ["Value", "Значення"],
  "Добавить": ["Add", "Додати"],
  "DayZ Override Builder — работает локально на вашем ПК; интернет нужен только для «Гайда по ссылке» и ИИ-помощника (по желанию). Не является продуктом Bohemia Interactive.": ["DayZ Override Builder — runs locally on your PC; internet is only needed for \"Guide by link\" and the AI assistant (optional). Not a Bohemia Interactive product.", "DayZ Override Builder — працює локально на вашому ПК; інтернет потрібен лише для «Гайда за посиланням» та ШІ-помічника (за бажанням). Не є продуктом Bohemia Interactive."],

  "cfgspawnabletypes и cfgrandompresets: проверка и слияние": ["cfgspawnabletypes and cfgrandompresets: check and merge", "cfgspawnabletypes і cfgrandompresets: перевірка та злиття"],
  "Спавн-лут": ["Spawn loot", "Спавн-лут"],
  "Спавн-лут — проверка cfgspawnabletypes + cfgrandompresets": ["Spawn loot — cfgspawnabletypes + cfgrandompresets check", "Спавн-лут — перевірка cfgspawnabletypes + cfgrandompresets"],
  "Проверяются: ссылки на пресеты (главная причина «пустых» зомби и рюкзаков), шансы вне 0..1, дубликаты типов и пресетов, пустые группы, quantmin/quantmax, неиспользуемые пресеты. Опционально приложите types.xml — сверю класснеймы.": ["Checks: preset references (the main cause of \"empty\" zombies and backpacks), chances outside 0..1, duplicate types and presets, empty groups, quantmin/quantmax, unused presets. Optionally attach types.xml to cross-check classnames.", "Перевіряються: посилання на пресети (головна причина «порожніх» зомбі та рюкзаків), шанси поза 0..1, дублікати типів і пресетів, порожні групи, quantmin/quantmax, невикористані пресети. Опційно прикріпіть types.xml — звірю класснейми."],
  "types.xml (необязательно)": ["types.xml (optional)", "types.xml (необов'язково)"],
  "Проверить спавн-лут": ["Check spawn loot", "Перевірити спавн-лут"],
  "Слияние спавн-лута двух серверов": ["Merging spawn loot of two servers", "Злиття спавн-луту двох серверів"],
  "Одинаковые записи пропускаются, новые типы и пресеты добавляются. При конфликте (один тип/пресет с разным содержимым) побеждает выбранная сторона, каждый конфликт — в отчёте.": ["Identical entries are skipped, new types and presets are added. On conflicts (same type/preset with different content) the chosen side wins; every conflict is reported.", "Однакові записи пропускаються, нові типи та пресети додаються. За конфлікту (один тип/пресет з різним вмістом) перемагає обрана сторона, кожен конфлікт — у звіті."],
  "ВАШ cfgspawnabletypes.xml": ["YOUR cfgspawnabletypes.xml", "ВАШ cfgspawnabletypes.xml"],
  "ВАШ cfgrandompresets.xml": ["YOUR cfgrandompresets.xml", "ВАШ cfgrandompresets.xml"],
  "НОВЫЙ cfgspawnabletypes.xml": ["NEW cfgspawnabletypes.xml", "НОВИЙ cfgspawnabletypes.xml"],
  "НОВЫЙ cfgrandompresets.xml": ["NEW cfgrandompresets.xml", "НОВИЙ cfgrandompresets.xml"],
  "При конфликте брать НОВЫЕ записи (иначе — мои)": ["On conflicts take NEW entries (otherwise mine)", "За конфлікту брати НОВІ записи (інакше — мої)"],
  "globals, economy, events, weather, gameplay и другие файлы миссии + день/ночь": ["globals, economy, events, weather, gameplay and other mission files + day/night", "globals, economy, events, weather, gameplay та інші файли місії + день/ніч"],
  "Миссия": ["Mission", "Місія"],
  "Файлы миссии — настройка с подсказками по каждой строчке": ["Mission files — editing with per-line hints", "Файли місії — налаштування з підказками до кожного рядка"],
  "Открыть файл": ["Open file", "Відкрити файл"],
  "⬇ Скачать изменённый файл": ["⬇ Download modified file", "⬇ Завантажити змінений файл"],
  "🌗 Калькулятор дня и ночи (serverDZ.cfg)": ["🌗 Day/night calculator (serverDZ.cfg)", "🌗 Калькулятор дня і ночі (serverDZ.cfg)"],
  "Светлое время в игре: с (час)": ["In-game daylight: from (hour)", "Світлий час у грі: з (година)"],
  "до (час)": ["to (hour)", "до (година)"],
  "Желаемый ДЕНЬ (реальных минут)": ["Desired DAY (real minutes)", "Бажаний ДЕНЬ (реальних хвилин)"],
  "Желаемая НОЧЬ (реальных минут)": ["Desired NIGHT (real minutes)", "Бажана НІЧ (реальних хвилин)"],
  "Рассчитать": ["Calculate", "Розрахувати"],
  "Поддерживаются: globals.xml, economy.xml, events.xml, cfgweather.xml, cfggameplay.json, cfgeffectarea.json, cfgplayerspawnpoints.xml, cfgeventspawns.xml (обзор). Откройте файл — под каждым параметром будет объяснение, что он делает. Правите значения → скачиваете готовый файл.": ["Supported: globals.xml, economy.xml, events.xml, cfgweather.xml, cfggameplay.json, cfgeffectarea.json, cfgplayerspawnpoints.xml, cfgeventspawns.xml (overview). Open a file — every parameter has an explanation of what it does. Edit values → download the ready file.", "Підтримуються: globals.xml, economy.xml, events.xml, cfgweather.xml, cfggameplay.json, cfgeffectarea.json, cfgplayerspawnpoints.xml, cfgeventspawns.xml (огляд). Відкрийте файл — під кожним параметром буде пояснення, що він робить. Правите значення → завантажуєте готовий файл."],
  "Задайте, сколько РЕАЛЬНОГО времени должны длиться игровой день и игровая ночь — программа посчитает ускорения времени для serverDZ.cfg. Светлое время суток в игре — примерно с 05:00 до 20:00 летом (15 ч); при желании поправьте.": ["Set how much REAL time the in-game day and night should last — the program computes time accelerations for serverDZ.cfg. In-game daylight is roughly 05:00–20:00 in summer (15 h); adjust if needed.", "Задайте, скільки РЕАЛЬНОГО часу мають тривати ігровий день і ніч — програма порахує прискорення часу для serverDZ.cfg. Світлий час у грі — приблизно з 05:00 до 20:00 влітку (15 год); за бажанням поправте."],
  "Вставьте эти строки в serverDZ.cfg. serverTimeAcceleration — ускорение игрового времени днём (1..64); serverNightTimeAcceleration — ДОПОЛНИТЕЛЬНЫЙ множитель ночью (0.1..64, итоговое ночное ускорение = произведение двух). serverTime=\"SystemTime\" — стартовое время с часов сервера; serverTimePersistent=0 — не сохранять время между рестартами.": ["Paste these lines into serverDZ.cfg. serverTimeAcceleration — daytime time acceleration (1..64); serverNightTimeAcceleration — ADDITIONAL night multiplier (0.1..64, total night acceleration = product of both). serverTime=\"SystemTime\" — start time from server clock; serverTimePersistent=0 — do not persist time between restarts.", "Вставте ці рядки в serverDZ.cfg. serverTimeAcceleration — прискорення ігрового часу вдень (1..64); serverNightTimeAcceleration — ДОДАТКОВИЙ множник вночі (0.1..64, підсумкове нічне прискорення = добуток двох). serverTime=\"SystemTime\" — стартовий час з годинника сервера; serverTimePersistent=0 — не зберігати час між рестартами."],
  "⚠ Программа может ошибаться. Перед заменой файлов на сервере: сделайте резервную копию оригинала, прочитайте во вкладке «Журнал», что именно сделала программа, и прогоните изменённый файл через «Проверку файлов». Особенно внимательно — с файлами миссии: ошибка в них может сломать экономику сервера.": ["⚠ The program can make mistakes. Before replacing files on the server: back up the original, read in the \"Log\" tab what exactly the program did, and run the modified file through \"File check\". Be especially careful with mission files: an error there can break the server economy.", "⚠ Програма може помилятися. Перед заміною файлів на сервері: зробіть резервну копію оригіналу, прочитайте у вкладці «Журнал», що саме зробила програма, і прожeніть змінений файл через «Перевірку файлів». Особливо уважно — з файлами місії: помилка в них може зламати економіку сервера."],
  "⚠ Проверьте рассчитанные значения на тестовом сервере перед боевым применением.": ["⚠ Verify the calculated values on a test server before production use.", "⚠ Перевірте розраховані значення на тестовому сервері перед бойовим застосуванням."],
  "⚠ Программа может ошибаться: делайте резервные копии, проверяйте полученные файлы перед использованием и читайте вкладку «Журнал» — там видно каждое действие.": ["⚠ The program can make mistakes: keep backups, verify generated files before use and read the \"Log\" tab — every action is visible there.", "⚠ Програма може помилятися: робіть резервні копії, перевіряйте отримані файли перед використанням і читайте вкладку «Журнал» — там видно кожну дію."],
  "Настройка мода InediaInfectedAI с подсказками из вики": ["InediaInfectedAI mod configuration with wiki hints", "Налаштування мода InediaInfectedAI з підказками з вікі"],
  "Настройка модов Terje (медицина, навыки и др.)": ["Terje mods configuration (medicine, skills etc.)", "Налаштування модів Terje (медицина, навички тощо)"],
  "Inedia Infected AI — настройка конфига": ["Inedia Infected AI — config editor", "Inedia Infected AI — налаштування конфіга"],
  "TerjeMods — настройка конфигов (медицина, навыки, радиация…)": ["TerjeMods — config editor (medicine, skills, radiation…)", "TerjeMods — налаштування конфігів (медицина, навички, радіація…)"],
  "Открыть выбранный файл": ["Open selected file", "Відкрити обраний файл"],
  "Поиск по параметрам…": ["Search parameters…", "Пошук за параметрами…"],
  "Открыть": ["Open", "Відкрити"],
  "⚠ Программа может ошибаться. Сделайте копию конфига, проверьте изменённый файл и вкладку «Журнал» перед заливкой на сервер.": ["⚠ The program can make mistakes. Back up the config, verify the modified file and the \"Log\" tab before uploading to the server.", "⚠ Програма може помилятися. Зробіть копію конфіга, перевірте змінений файл і вкладку «Журнал» перед заливанням на сервер."],
  "или путь:": ["or path:", "або шлях:"],
  "Protection/biological  или  weight": ["Protection/biological  or  weight", "Protection/biological  або  weight"],

  // динамические строки интерфейса (из app.js)
  "Класснейм": ["Classname", "Класснейм"],
  "Категория": ["Category", "Категорія"],
  "Название": ["Name", "Назва"],
  "Было": ["Was", "Було"],
  "Стало": ["Now", "Стало"],
  "Родитель": ["Parent", "Батько"],
  "ожидание…": ["waiting…", "очікування…"],
  "Скопировано ✓": ["Copied ✓", "Скопійовано ✓"],

  // ---- v1.10: ИИ-помощник и «Гайд по ссылке» ----
  "ИИ-помощник: разбор гайдов и перевод описаний (бесплатные нейросети)": ["AI assistant: guide parsing and description translation (free AI providers)", "ШІ-помічник: розбір гайдів і переклад описів (безкоштовні нейромережі)"],
  "🤖 ИИ-помощник (бесплатные нейросети)": ["🤖 AI assistant (free AI providers)", "🤖 ШІ-помічник (безкоштовні нейромережі)"],
  "Разбирает гайды со свободным текстом и переводит описания параметров на русский. Работает через бесплатные тарифы нейросетей — нужен только API-ключ (получается за пару минут, банковская карта не нужна). Ключ хранится ТОЛЬКО на вашем компьютере (settings.json). Нейросети отправляется текст гайда и имена параметров — сами значения ваших конфигов не отправляются.": ["Parses free-form guides and translates parameter descriptions. Uses free AI provider tiers — you only need an API key (takes a couple of minutes, no bank card). The key is stored ONLY on your computer (settings.json). Only the guide text and parameter names are sent to the AI — the values of your configs are not sent.", "Розбирає гайди з вільним текстом і перекладає описи параметрів. Працює через безкоштовні тарифи нейромереж — потрібен лише API-ключ (отримується за кілька хвилин, банківська картка не потрібна). Ключ зберігається ЛИШЕ на вашому комп'ютері (settings.json). Нейромережі надсилається текст гайда та імена параметрів — самі значення ваших конфігів не надсилаються."],
  "Включить ИИ-помощник": ["Enable AI assistant", "Увімкнути ШІ-помічника"],
  "Провайдер (пресеты бесплатных)": ["Provider (free presets)", "Провайдер (безкоштовні пресети)"],
  "Адрес API (base URL)": ["API address (base URL)", "Адреса API (base URL)"],
  "API-ключ": ["API key", "API-ключ"],
  "вставьте ключ провайдера": ["paste the provider key", "вставте ключ провайдера"],
  "Модель": ["Model", "Модель"],
  "имя модели": ["model name", "ім'я моделі"],
  "Запросить у провайдера список доступных моделей": ["Ask the provider for the list of available models", "Запитати у провайдера список доступних моделей"],
  "Список моделей": ["List models", "Список моделей"],
  "Проверить подключение": ["Test connection", "Перевірити підключення"],
  "⚠ ИИ может ошибаться — сверяйте подставленные описания с оригинальным гайдом. Бесплатные тарифы имеют лимиты: при ошибке «429» подождите минуту или смените провайдера.": ["⚠ AI can make mistakes — verify inserted descriptions against the original guide. Free tiers have limits: on error \"429\" wait a minute or switch provider.", "⚠ ШІ може помилятися — звіряйте підставлені описи з оригінальним гайдом. Безкоштовні тарифи мають ліміти: за помилки «429» зачекайте хвилину або змініть провайдера."],
  "📖 Гайд по ссылке": ["📖 Guide by link", "📖 Гайд за посиланням"],
  "— описания и ответы из гайда мода": ["— descriptions and answers from the mod's guide", "— описи та відповіді з гайда мода"],
  "Вставьте ссылку на гайд (github wiki, страница или папка WIKI) — программа скачает его и подставит описания к параметрам открытого файла. Режим «через ИИ» дополнительно переводит описания на русский и понимает свободный текст (настройка — кнопка 🤖 вверху). Гайд кэшируется: повторная загрузка не тратит интернет.": ["Paste a guide link (github wiki, page or WIKI folder) — the program downloads it and attaches descriptions to the parameters of the opened file. The \"via AI\" mode additionally translates descriptions and understands free-form text (configure via the 🤖 button above). The guide is cached: repeat loads don't use the internet.", "Вставте посилання на гайд (github wiki, сторінка або тека WIKI) — програма завантажить його та підставить описи до параметрів відкритого файлу. Режим «через ШІ» додатково перекладає описи та розуміє вільний текст (налаштування — кнопка 🤖 вгорі). Гайд кешується: повторні завантаження не витрачають інтернет."],
  "Вставьте ссылку на гайд (github wiki или страница) — программа скачает его, покажет справочник «параметр → описание» и ответит на вопросы по гайду через ИИ (настройка — кнопка 🤖 вверху). Гайд кэшируется.": ["Paste a guide link (github wiki or page) — the program downloads it, shows a \"parameter → description\" reference and answers questions about the guide via AI (configure via the 🤖 button above). The guide is cached.", "Вставте посилання на гайд (github wiki або сторінка) — програма завантажить його, покаже довідник «параметр → опис» і відповість на питання щодо гайда через ШІ (налаштування — кнопка 🤖 вгорі). Гайд кешується."],
  "структурный разбор": ["structural parsing", "структурний розбір"],
  "через ИИ (перевод на русский)": ["via AI (with translation)", "через ШІ (з перекладом)"],
  "Загрузить гайд": ["Load guide", "Завантажити гайд"],
  "Скачать заново, минуя кэш": ["Re-download, bypassing the cache", "Завантажити заново, оминаючи кеш"],
  "Извлечённый справочник (параметр → описание)": ["Extracted reference (parameter → description)", "Витягнутий довідник (параметр → опис)"],
  "Фильтр по справочнику…": ["Filter the reference…", "Фільтр довідника…"],
  "Вопрос по гайду, например: как сделать зомби медленнее?": ["A question about the guide, e.g.: how do I make zombies slower?", "Питання щодо гайда, наприклад: як зробити зомбі повільнішими?"],
  "🤖 Спросить": ["🤖 Ask", "🤖 Запитати"],
  "Groq — быстрый, до 1000+ запросов/день бесплатно": ["Groq — fast, up to 1000+ free requests/day", "Groq — швидкий, до 1000+ безкоштовних запитів/день"],
  "OpenRouter — модели с пометкой :free": ["OpenRouter — models tagged :free", "OpenRouter — моделі з позначкою :free"],
  "Google Gemini (AI Studio) — щедрый бесплатный лимит": ["Google Gemini (AI Studio) — generous free tier", "Google Gemini (AI Studio) — щедрий безкоштовний ліміт"],
  "GitHub Models — по GitHub-аккаунту": ["GitHub Models — via your GitHub account", "GitHub Models — за GitHub-акаунтом"],
  "Локальный Ollama — без интернета и ключей": ["Local Ollama — no internet or keys", "Локальний Ollama — без інтернету та ключів"],
  "Свой сервер (OpenAI-совместимый API)": ["Custom server (OpenAI-compatible API)", "Свій сервер (OpenAI-сумісний API)"],
  "Ключ: console.groq.com/keys (регистрация, карта не нужна).": ["Key: console.groq.com/keys (registration, no card needed).", "Ключ: console.groq.com/keys (реєстрація, картка не потрібна)."],
  "Ключ: openrouter.ai/settings/keys. Бесплатно ~50 запросов/день. Выбирайте модели с «:free».": ["Key: openrouter.ai/settings/keys. Free ~50 requests/day. Pick models tagged \":free\".", "Ключ: openrouter.ai/settings/keys. Безкоштовно ~50 запитів/день. Обирайте моделі з «:free»."],
  "Ключ: aistudio.google.com/apikey (Google-аккаунт). Из некоторых стран API недоступен напрямую.": ["Key: aistudio.google.com/apikey (Google account). In some countries the API is not directly available.", "Ключ: aistudio.google.com/apikey (Google-акаунт). З деяких країн API недоступний напряму."],
  "Ключ: github.com/settings/tokens → Fine-grained token с разрешением «Models: read».": ["Key: github.com/settings/tokens → fine-grained token with \"Models: read\" permission.", "Ключ: github.com/settings/tokens → fine-grained token із дозволом «Models: read»."],
  "Нужен установленный Ollama (ollama.com) и скачанная модель. Ключ не требуется.": ["Requires installed Ollama (ollama.com) and a downloaded model. No key needed.", "Потрібен встановлений Ollama (ollama.com) і завантажена модель. Ключ не потрібен."],
  "Любой сервис с OpenAI-совместимым API: адрес, ключ и имя модели.": ["Any service with an OpenAI-compatible API: address, key and model name.", "Будь-який сервіс з OpenAI-сумісним API: адреса, ключ та ім'я моделі."],
};

// ---------- движок ----------
let curLang = localStorage.getItem("dob_lang") || "ru";
let enabledLangs = JSON.parse(localStorage.getItem("dob_langs") || '["ru","en","uk"]');
if (!enabledLangs.includes(curLang)) curLang = enabledLangs[0] || "ru";

function i18nGet(s) {
  if (curLang === "ru") return null;
  const e = I18N[s];
  if (!e) return null;
  return curLang === "en" ? e[0] : e[1];
}

function translateTextNode(node) {
  if (node.__orig === undefined) node.__orig = node.data;
  const key = node.__orig.trim().replace(/\s+/g, " ");
  if (!key) return;
  const tr = i18nGet(key);
  const target = tr !== null ? tr : node.__orig;
  if (node.data !== target) node.data = target;
}

function translateAttrs(el) {
  ["placeholder", "title"].forEach((a) => {
    if (!el.hasAttribute || !el.hasAttribute(a)) return;
    if (!el.__origAttrs) el.__origAttrs = {};
    if (el.__origAttrs[a] === undefined) el.__origAttrs[a] = el.getAttribute(a);
    const tr = i18nGet(el.__origAttrs[a].trim());
    el.setAttribute(a, tr !== null ? tr : el.__origAttrs[a]);
  });
}

function translateTree(root) {
  const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT | NodeFilter.SHOW_ELEMENT, null);
  let n = root.nodeType === 3 ? root : walker.nextNode();
  if (root.nodeType === 1) translateAttrs(root);
  if (root.nodeType === 3) translateTextNode(root);
  while (n) {
    if (n.nodeType === 3) translateTextNode(n);
    else if (n.nodeType === 1 && n.tagName !== "SCRIPT" && n.tagName !== "STYLE") translateAttrs(n);
    n = walker.nextNode();
  }
}

function applyLang(lang) {
  curLang = lang;
  localStorage.setItem("dob_lang", lang);
  document.documentElement.lang = lang;
  translateTree(document.body);
}

// перевод динамически добавляемых элементов (таблицы, чипы и т.п.)
const i18nObserver = new MutationObserver((muts) => {
  if (curLang === "ru") return;
  muts.forEach((m) => {
    m.addedNodes.forEach((n) => {
      if (n.nodeType === 3 || n.nodeType === 1) translateTree(n);
    });
  });
});

// ---------- переключатель и управление списком языков ----------
function i18nBuildSwitcher() {
  const sel = document.getElementById("langSel");
  if (!sel) return;
  sel.innerHTML = "";
  enabledLangs.forEach((l) => {
    const o = document.createElement("option");
    o.value = l;
    o.textContent = I18N_LANGS[l] || l;
    if (l === curLang) o.selected = true;
    sel.appendChild(o);
  });
}

function i18nInit() {
  i18nBuildSwitcher();
  const sel = document.getElementById("langSel");
  if (sel) sel.addEventListener("change", () => applyLang(sel.value));
  const cfg = document.getElementById("langCfgBtn");
  if (cfg) {
    cfg.addEventListener("click", () => {
      const names = { ru: "Русский", en: "English", uk: "Українська" };
      const answer = prompt(
        (curLang === "en"
          ? "Enabled languages (comma-separated codes): ru, en, uk.\nRemove the ones you don't need:"
          : curLang === "uk"
            ? "Увімкнені мови (коди через кому): ru, en, uk.\nПриберіть непотрібні:"
            : "Включённые языки (коды через запятую): ru, en, uk.\nУберите ненужные:"),
        enabledLangs.join(", ")
      );
      if (answer === null) return;
      const list = answer.split(/[,;\s]+/).map((s) => s.trim().toLowerCase()).filter((s) => names[s]);
      if (!list.length) {
        alert(curLang === "en" ? "At least one language must remain" : curLang === "uk" ? "Має лишитися хоча б одна мова" : "Должен остаться хотя бы один язык");
        return;
      }
      enabledLangs = [...new Set(list)];
      localStorage.setItem("dob_langs", JSON.stringify(enabledLangs));
      if (!enabledLangs.includes(curLang)) applyLang(enabledLangs[0]);
      i18nBuildSwitcher();
    });
  }
  if (curLang !== "ru") applyLang(curLang);
  i18nObserver.observe(document.body, { childList: true, subtree: true });
}

document.addEventListener("DOMContentLoaded", i18nInit);
