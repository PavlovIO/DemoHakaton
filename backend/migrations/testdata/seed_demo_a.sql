-- Вариант A сида BE-16 — KYC с нуля: применяется после seed_demo_common.sql и перед
-- seed_demo_kyc.sql (make db-reset SEED=a). «Альфа» остаётся draft без документов
-- (загрузку и submitKyc показывают вживую), Beta уже подала KYC и ждёт решения админа.
-- С вариантом B на одной БД несовместим: смена варианта — только через make db-reset.

-- 1. Beta → under_review. Охрана статуса в WHERE: повторный сид ничего не меняет.
UPDATE organizations SET kyc_status = 'under_review', kyc_submitted_at = now() - interval '1 day'
WHERE id = '22222222-2222-2222-2222-222222222222' AND kyc_status = 'draft';

-- 2. Документы Beta — полный комплект кредитора (как требует submitKyc).
INSERT INTO kyc_documents (organization_id, doc_type, storage_key, file_name, mime_type, size_bytes, sha256,
                           uploaded_by_user_id, uploaded_at)
SELECT d.organization_id::uuid, d.doc_type::kyc_document_type, d.storage_key, d.file_name, 'application/pdf',
       f.size_bytes, decode(f.sha256, 'hex'), u.id, o.kyc_submitted_at - interval '1 hour'
FROM (VALUES
    ('22222222-2222-2222-2222-222222222222', 'charter', 'de000000000000000000000000000201', 'Charter_Beta.pdf', 'demo_document.pdf'),
    ('22222222-2222-2222-2222-222222222222', 'registration_certificate', 'de000000000000000000000000000202', 'RegistryExtract_Beta.pdf', 'demo_registry_extract.pdf'),
    ('22222222-2222-2222-2222-222222222222', 'address_proof', 'de000000000000000000000000000203', 'AddressProof_Beta.pdf', 'demo_document.pdf'),
    ('22222222-2222-2222-2222-222222222222', 'beneficiaries_register', 'de000000000000000000000000000204', 'Beneficiaries_Beta.pdf', 'demo_document.pdf'),
    ('22222222-2222-2222-2222-222222222222', 'source_of_funds', 'de000000000000000000000000000205', 'SourceOfFunds_Beta.pdf', 'demo_document.pdf')
) AS d(organization_id, doc_type, storage_key, file_name, file)
JOIN demo_files f ON f.file = d.file
JOIN organizations o ON o.id = d.organization_id::uuid
JOIN users u ON u.organization_id = o.id
ON CONFLICT (storage_key) DO NOTHING;

-- 3. Снапшот мок-реестра, как пишет submitKyc; историю, аудит и уведомление админу —
-- seed_demo_kyc.sql.
INSERT INTO registry_checks (organization_id, source, query, found, name, registration_number, legal_address,
                             registry_status, raw_response, checked_at)
SELECT o.id, 'mock', o.lei, true, o.name, o.registration_number, o.legal_address, 'active', '{}'::jsonb, o.kyc_submitted_at
FROM organizations o
WHERE o.id = '22222222-2222-2222-2222-222222222222'
  AND NOT EXISTS (SELECT 1 FROM registry_checks r WHERE r.organization_id = o.id);
