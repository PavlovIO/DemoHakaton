-- +goose Up
-- +goose StatementBegin
-- Серый список FATF (increased monitoring) по итогам пленара 19.06.2026 — 22 юрисдикции
-- (DECISIONS, «Факты из первоисточников»): без него правило FATF grey → medium (BE-13)
-- на демо не срабатывает, а Delta Trading (KW, BE-16) не может быть зарегистрирована.
-- Источник: https://www.fatf-gafi.org/en/publications/High-risk-and-other-monitored-jurisdictions/increased-monitoring-june-2026.html
-- Полный справочник ISO 3166-1 — BE-02; его вставки стран должны быть ON CONFLICT.
-- is_unfriendly — по перечню 430-р (DECISIONS п. 35): BG и MC — государства перечня, VG —
-- британская заморская территория.
INSERT INTO countries (code, name_ru, name_en, fatf_list, fatf_as_of, is_unfriendly) VALUES
    ('AO', 'Ангола',                       'Angola',                           'grey', '2026-06-19', false),
    ('BO', 'Боливия',                      'Bolivia',                          'grey', '2026-06-19', false),
    ('BA', 'Босния и Герцеговина',         'Bosnia and Herzegovina',           'grey', '2026-06-19', false),
    ('BG', 'Болгария',                     'Bulgaria',                         'grey', '2026-06-19', true),
    ('CM', 'Камерун',                      'Cameroon',                         'grey', '2026-06-19', false),
    ('CI', 'Кот-д’Ивуар',                  'Côte d''Ivoire',                   'grey', '2026-06-19', false),
    ('CD', 'ДР Конго',                     'Democratic Republic of the Congo', 'grey', '2026-06-19', false),
    ('HT', 'Гаити',                        'Haiti',                            'grey', '2026-06-19', false),
    ('IQ', 'Ирак',                         'Iraq',                             'grey', '2026-06-19', false),
    ('KE', 'Кения',                        'Kenya',                            'grey', '2026-06-19', false),
    ('KW', 'Кувейт',                       'Kuwait',                           'grey', '2026-06-19', false),
    ('LA', 'Лаос',                         'Laos',                             'grey', '2026-06-19', false),
    ('LB', 'Ливан',                        'Lebanon',                          'grey', '2026-06-19', false),
    ('MC', 'Монако',                       'Monaco',                           'grey', '2026-06-19', true),
    ('NP', 'Непал',                        'Nepal',                            'grey', '2026-06-19', false),
    ('PG', 'Папуа — Новая Гвинея',         'Papua New Guinea',                 'grey', '2026-06-19', false),
    ('SS', 'Южный Судан',                  'South Sudan',                      'grey', '2026-06-19', false),
    ('SY', 'Сирия',                        'Syria',                            'grey', '2026-06-19', false),
    ('VE', 'Венесуэла',                    'Venezuela',                        'grey', '2026-06-19', false),
    ('VN', 'Вьетнам',                      'Vietnam',                          'grey', '2026-06-19', false),
    ('VG', 'Британские Виргинские острова', 'British Virgin Islands',          'grey', '2026-06-19', true),
    ('YE', 'Йемен',                        'Yemen',                            'grey', '2026-06-19', false)
ON CONFLICT (code) DO UPDATE SET fatf_list = EXCLUDED.fatf_list, fatf_as_of = EXCLUDED.fatf_as_of;

-- Каналы платежей из РФ в обход SWIFT (DECISIONS п. 24); фильтр витрины — BE-07.
-- ON CONFLICT — как у стран: справочник BE-02 может завести их раньше.
INSERT INTO transfer_methods (code, name) VALUES
    ('spfs', 'СПФС Банка России'),
    ('cips', 'CIPS (Китай)')
ON CONFLICT (code) DO NOTHING;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- Строки, на которые ссылаются данные (демо-сид: Delta в KW, заявки и сделки через
-- spfs/cips), остаются: иначе откат на заселённой БД упал бы на FK. Отметку FATF
-- снимаем у всех — без миграции серого списка нет.
DELETE FROM transfer_methods t
WHERE t.code IN ('spfs', 'cips')
  AND NOT EXISTS (SELECT 1 FROM applications a WHERE a.transfer_method = t.code)
  AND NOT EXISTS (SELECT 1 FROM deals d WHERE d.transfer_method = t.code);
UPDATE countries SET fatf_list = NULL, fatf_as_of = NULL
WHERE code IN ('AO', 'BO', 'BA', 'BG', 'CM', 'CI', 'CD', 'HT', 'IQ', 'KE', 'KW',
               'LA', 'LB', 'MC', 'NP', 'PG', 'SS', 'SY', 'VE', 'VN', 'VG', 'YE');
-- VN был в справочнике до этой миграции и не удаляется.
DELETE FROM countries c
WHERE c.code IN ('AO', 'BO', 'BA', 'BG', 'CM', 'CI', 'CD', 'HT', 'IQ', 'KE', 'KW',
                 'LA', 'LB', 'MC', 'NP', 'PG', 'SS', 'SY', 'VE', 'VG', 'YE')
  AND NOT EXISTS (SELECT 1 FROM organizations o WHERE o.country_code = c.code)
  AND NOT EXISTS (SELECT 1 FROM beneficiaries b WHERE b.citizenship_country_code = c.code)
  AND NOT EXISTS (SELECT 1 FROM applications a WHERE a.target_country_code = c.code)
  AND NOT EXISTS (SELECT 1 FROM deals d WHERE d.target_country_code = c.code);
-- +goose StatementEnd
