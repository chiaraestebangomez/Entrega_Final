INSERT INTO itinerary.agencies (id, name, email, phone, description) VALUES
    ('11111111-1111-4111-8111-111111111111', 'Viajes Horizonte', 'contacto@horizonte.travel', '+54 11 5555-1000', 'Agencia especializada en viajes a medida por Sudamérica.'),
    ('22222222-2222-4222-8222-222222222222', 'Global Trip Partners', 'hola@globaltrip.example', '+34 91 555 2000', 'Agencia internacional con foco en Europa y Norteamérica.'),
    ('33333333-3333-4333-8333-333333333333', 'Costa Azul Viajes', 'info@costaazul.example', '+54 223 555-3000', 'Escapadas de playa, cruceros y experiencias gastronómicas.')
ON CONFLICT (id) DO NOTHING;

INSERT INTO itinerary.agency_recommendations
    (id, agency_id, title, destination, description, price_cents, currency, start_date, end_date, active) VALUES
    ('a1111111-1111-4111-8111-111111111111', '11111111-1111-4111-8111-111111111111', 'Patagonia esencial', 'Bariloche', 'Siete noches con navegación y trekking guiado.', 129900, 'USD', CURRENT_DATE + 45, CURRENT_DATE + 52, true),
    ('a2222222-2222-4222-8222-222222222222', '11111111-1111-4111-8111-111111111111', 'Ruta del vino', 'Mendoza', 'Cuatro días de bodegas y gastronomía.', 79900, 'USD', CURRENT_DATE + 30, CURRENT_DATE + 34, true),
    ('a3333333-3333-4333-8333-333333333333', '22222222-2222-4222-8222-222222222222', 'Madrid y Barcelona', 'Madrid', 'Ocho noches con trenes y museos incluidos.', 189900, 'USD', CURRENT_DATE + 60, CURRENT_DATE + 68, true),
    ('a4444444-4444-4444-8444-444444444444', '22222222-2222-4222-8222-222222222222', 'Nueva York clásico', 'Nueva York', 'Cinco noches en Manhattan con city pass.', 159900, 'USD', CURRENT_DATE + 90, CURRENT_DATE + 95, true),
    ('a5555555-5555-4555-8555-555555555555', '33333333-3333-4333-8333-333333333333', 'Caribe todo incluido', 'Miami', 'Crucero de cinco noches con salida desde Miami.', 109900, 'USD', CURRENT_DATE + 75, CURRENT_DATE + 80, true),
    ('a6666666-6666-4666-8666-666666666666', '33333333-3333-4333-8333-333333333333', 'Lisboa y Sintra', 'Lisboa', 'Seis noches con excursiones y desayuno.', 99900, 'USD', CURRENT_DATE + 100, CURRENT_DATE + 106, true)
ON CONFLICT (id) DO NOTHING;
