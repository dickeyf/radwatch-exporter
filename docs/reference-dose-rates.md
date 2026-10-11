# Reference dose rates for dashboards

Reference lines for the dose-rate panel (IMPL-31), to put readings in perspective. `radwatch_dose_rate_sieverts_per_hour` is in Sv/h, so the **Sv/h** column is what goes into a Grafana threshold or a constant query. Plot on a **log scale**: these span about nine orders of magnitude.

All values are approximate, rounded and from general literature; ranges vary by place, altitude, season and instrument. A GM counter measures external gamma (and some beta), so it shows less than the "total background" figures that include inhaled radon.

| Reference | Sv/h | Notes |
|-----------|------|-------|
| World average external background | 1e-7 | ~0.06–0.1 µSv/h outdoor gamma (terrestrial + cosmic). Total natural dose incl. radon is ~2.4 mSv/year (~0.27 µSv/h equivalent), most of it not visible to a GM counter. |
| Local background (measured) | 1e-7 | Replace with your own long-term median, e.g. `quantile_over_time(0.5, radwatch_dose_rate_sieverts_per_hour[30d])`. The reference counter averages ~0.10 µSv/h. |
| Public dose limit, 1 mSv/year as a rate | 1.1e-7 | 1 mSv / 8,766 h ≈ 0.11 µSv/h **above** background (artificial sources). Draw as background + 1.1e-7. |
| Kerala coast, India (monazite sands) | 1e-6 – 4e-6 | ~1–4 µSv/h in the high-background villages. |
| Flight at ~35,000 ft | 3e-6 – 6e-6 | ~3–6 µSv/h at cruise, higher near the poles and at solar maximum. |
| Occupational limit, 20 mSv/year over 2,000 working hours | 1e-5 | ~10 µSv/h, the average rate of a radiation worker at the limit. |
| Guarapari beach, Brazil (hotspots) | 1e-5 – 2e-5 | ~10–20 µSv/h on the darkest monazite sand. |
| International Space Station | 2e-5 – 4e-5 | ~0.5–1 mSv/day. |
| Ramsar, Iran (Talesh Mahalleh, highest known natural background) | 1e-5 – 3e-5 | Up to ~260 mSv/year in a few homes, ~30 µSv/h at the worst spots. |
| Chernobyl exclusion zone today (Red Forest hotspots) | 1e-5 – 1e-4 | ~10–100 µSv/h at the worst spots; Pripyat streets ~1 µSv/h or less. |
| Reference counter's practical ceiling | 1e-3 | The GMC-500+ warns "beyond range" just above ~1 mSv/h (measured with natural uranium ore). Readings near this line are unreliable. |
| **"3.6 roentgen: not great, not terrible"** | 3.6e-2 | 3.6 R/h ≈ 32–36 mSv/h (1 R ≈ 8.8 mGy in air, often rounded to 10 mSv). In the story, the dosimeter's maximum, not the real level. |
| Chernobyl reactor roof, 1986 ("Masha" zone, graphite clearing by liquidators) | 10 – 100 | Commonly cited 1,000–10,000+ R/h at the worst spots near ejected graphite and fuel; liquidators worked shifts of tens of seconds to ~2 minutes. |
| Elephant's Foot, Chernobyl, 1986 | ~100 | ~10,000 R/h when first measured: a lethal dose in a few minutes. |

For scale, as doses rather than rates: a chest X-ray is ~0.1 mSv, a CT scan ~2–10 mSv, the annual occupational limit 20 mSv, and an acute whole-body dose of ~4–5 Sv is lethal for about half of those exposed without treatment.
