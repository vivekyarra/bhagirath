# Backtest evidence base: research notes

Collected 2026-11-04. Every `quote` in the four CSVs was checked by script (`../tools/build.py`) against the text of the page as I downloaded it, with whitespace and typographic quotes normalised. All quotes matched except one, which I checked by hand: IND-001 (CAG). The PDF prints footnote markers inside that sentence ("3,074 28 physical, 147 29 chemical…"), and I removed them from the quote.

Raw HTML/PDF copies are in `raw/` and the extracted text is in `raw/txt/`.

Conventions:
- A blank `date_earliest` means the source gives no lower bound. Such rows are undated in the source.
- `count` is a lower bound wherever the source says "more than" or "over". The notes say so.
- Cumulative figures and current in-patient (stock) figures are labelled in the notes.
- Hindi quotes are verbatim, and a translation is in the notes.

---

## 1. Indore, Bhagirathpura (`indore_bhagirathpura_2025.csv`, 72 rows)

### What I searched
- The English and Hindi press. Queries covered first death, first admissions, the 311 helpline and CM Helpline complaints, the Oct/Nov/Dec complaint timeline, MGM/MPPCB/Aurobindo sample reports, HC status reports, the death audit and the judicial (Justice Sushil Kumar Gupta) commission report.
- The MP High Court website for the orders and the commission report. I could not reach it (see failed URLs).

### Key dates

| Signal | Date |
|---|---|
| Earliest dated complaint | 2024-02-29: councillor's letter to the Mayor about people falling sick from dirty water |
| Earliest dated helpline complaint | 2025-10-15: "borewell water is mixing with drain water" |
| Later dated complaints | mid-Nov 2025 ("acid in the dirty water"); 2025-12-18 ("foul stench" in Narmada supply) |
| Pre-outbreak tests | City-wide only: CAG 2013-18 (827 adverse bacteriological IMC samples); CAG joint sampling Aug–Sep 2018 (5 of 24 IMC samples with faecal coliform 40–140 vs 0). Bhagirathpura is not named in either. The commission's "2025 municipal records: 73 samples, 54 failed, 39 faecal coliform" has no dates or location. |
| First post-outbreak test | Sampled ~2025-12-29: 26 of 50 positive for bacteria (derived from "four days ago" on 2 Jan) |
| Illness onset | 2025-12-24 (Aaj Tak, ANI resident). FPJ cremation records claim a diarrhoea death on 21 Dec, with illness from ~9–11 Dec. |
| First hospitalisation | 2025-12-25 (one family account, Ground Report); 28 Dec (one family, ETV); 29 Dec (>35 admitted, Aaj Tak). Officials later dated the outbreak start to 29 Dec. |
| First death | Claimed 21 Dec (FPJ, cremation records) and 24 Dec ([name withheld], recorded as "old age"). Corroborated named deaths on 29 Dec ([name withheld], [name withheld], infant [name withheld]). |

### What I could NOT find
- The individual 311 or CM Helpline complaint records with dates and counts per day. The 11 CM Helpline complaints from the ward (Bhaskar) are undated. The 266/23 Zone-4 figure is for the whole year.
- The Indian Express original behind the Oct 15 / mid-Nov / Dec 18 timeline. I used The Live Nagpur (English rework, "according to a media report") and The Lallantop (Hindi rework citing IE).
- A dated pre-outbreak water test for Bhagirathpura.
- The text of the judicial commission report (MPHC site unreachable). I used PTI/IND24/The Wire summaries.
- The original NDTV article ("1 dead, over 60 fall ill…"). It returned 403 (Akamai), WebFetch was blocked, and web.archive.org reset the connection. **None of its content is used.**
- The claim "residents complained water supplied on December 25 had an unusual smell" (councillor, Deccan Herald). It appeared only in a search-engine summary. The live DH page I opened has been updated and no longer contains it, so **it is not used.**
- The claim "[name withheld] developed diarrhoea on December 27" appeared only in a search summary of a page I did not open, so it is not used. Millennium Post (opened) says vomiting began on the evening of 28 Dec.

### Conflicts between sources
- **Date of the first helpline complaint:** Live Nagpur says "October 15". Lallantop says "तारीख 15 दिसंबर" (15 Dec) but in the same passage calls it "two months before" and then "by mid-November". 15 Dec looks like a translation error. I recorded 15 Oct and flagged it.
- **Outbreak start:**
  - Media and residents: 24 Dec (Aaj Tak); "past nine days" as of 2 Jan (PTI); "eight days since the outbreak" as of 1 Jan (health official).
  - Officials (PTI, 12 Jan): "began on December 29". The Wire (Oct 2026) repeats 29 Dec.
  - NGT petition (ThePrint, 15 Jan): "last week of December 2025".
- **Death toll:**
  - 31 Dec: health department 3; DM 4; Mayor 7; AIR 5; residents ≥8.
  - 2 Jan: Mayor 10; state HC status report 4.
  - 5 Jan: Divisional Commissioner 7 (6 the previous day); locals 17.
  - 15 Jan: HC status report 7. MGM death audit: 15 "linked in some way".
  - 28 Jan: state death audit says 16 of 23 (HC questioned its basis).
  - Compensation was paid to 18 families (~8–12 Jan) and then 21 families (by 16 Jan).
  - Local and unofficial counts: 24, 25, 27 (FPJ, 24 Jan), 32 (ETV, 2 Feb), 36 (residents/Congress).
  - Commission: Health Dept informed of 35 deaths by 12 Feb, 22 "possibly related" (PTI and The Wire). ETV Bharat and Logical Indian (Aug–Sep 2026, citing lawyers before publication) said 24 of 36.
- **Hospital snapshot on 5 Jan:** 110 current (PTI) vs 142 current (Deccan Chronicle).
- **Water tests:**
  - Positive: 26 of 50 (~29 Dec sampling); 35 of 51 faecal coliform (DC, 5 Jan); 35 of 60 failed (MGM, 8 Jan); MPPCB 3 Jan: 50 of 52 total coliform, 40 of 52 E. coli; 51 tube wells with E. coli (Chief Secretary, 20 Jan).
  - Negative: Sri Aurobindo RT-PCR report dated 4 Jan found all 5 samples negative. This was after chlorination, and the report itself asked for culture confirmation.
  - Several of these batches probably overlap. **Do not sum them.**
- **Symptomatic counts on 31 Dec:** 2,456 suspected (CM) vs 2,756 symptomatic (FPJ).
- **Age of the infant victim:** six months (father, PTI 1 Jan) vs "five-month-old" (state status report).

---

## 2. Shimla hepatitis E, 2015–16 (`shimla_2015.csv`, 22 rows)

### What I searched
Tribune archive, Down To Earth, Himachal Watcher, Wikipedia, the IDSP-based outbreak paper (IJAR 2018), the IGMC water bacteriology paper (IJCMPH 2017), and searches for pre-outbreak test failures, SIT/HC reports and the first death.

### Key dates

| Signal | Date |
|---|---|
| IDSP signal | Dec 2015 (day not given) |
| Index case onset | Second week of Dec 2015 |
| Rise in cases | "Last week of December" |
| STP inspection | 31 Dec 2015 |
| Intake inspection and supply stopped | 2 Jan 2016 |
| NIV sampling | 28–31 Jan 2016; HEV RNA in 8 of 8 samples (report 10 Feb) |
| Death count | One death reported by 20 Jan 2016 (exact date unknown) |

### What I could NOT find
- Any dated resident complaint. The only complaint evidence is "complaints of turbid water supply were received" (IJAR, undated) and the Deputy Mayor's "several complaints to the IPH department" about the STPs (DTE, undated).
- Any pre-outbreak lab test. Labs tested only coliform and chlorine; there was no virology lab. Coliform data in IJCMPH start on 18 Jan 2016.
- The date of the first death. A Tribune article titled "Jaundice claims its first victim in state" turned out to be from **January 2018** (a girl from Sundernagar), not this outbreak, so I excluded it.
- The HP High Court and SIT reports.
- Medindia (403).

### Conflicts
- **Onset:** Dec 2015 (IDSP paper, Tribune, Himachal Watcher) vs "outbreak started in mid-January" (DTE).
- **Deaths:** minister said 4 (23 Feb) vs Tribune's tally of 9 the same day; minister said 10 Himachal-wide (24 Feb); IJAR paper's final figure is 33.
- **Cases:** 1,620 (Tribune, 3 Mar) vs 1,681 HEV/HAV cases of more than 6,000 jaundice cases (IJAR).
- **IJAR internal error:** it prints the period as "11th December 2015 to 25th May 2015"; the end date must be 2016.
- **IJCMPH internal inconsistency:** 129 of 1,098 unsatisfactory, but also "all samples were found contaminated" in Jan–Apr.

### Caveat
The illness signal here is jaundice (hepatitis E), not diarrhoea or vomiting, so the `diarrhoea` column is blank throughout.

---

## 3. Gandhinagar typhoid, 2025–26 (`gandhinagar_2026.csv`, 14 rows)

### What I searched
PTI/ANI/AIR copy, Deccan Herald, The Wire, Bombay Samachar, MedBound Times, Navbharat, plus English, Hindi and Gujarati queries for dated complaints.

### What I could NOT find
- Dated or counted resident complaints. One outlet (MedBound) says foul or discoloured water was reported "days before people started falling sick".
- Any pre-outbreak test.
- A first-admission date.
- Final totals after 8 Jan 2026.
- Pages I could not open: The Wire desktop page (no body text; the mobile page worked), OpIndia (403), DNA (403), The Week (403).

### Conflicts
- **Water tests:** the hospital superintendent said samples "tested positive" (~3 Jan); the Deputy CM said 367 of 367 samples taken on 4 Jan were satisfactory; the Municipal Commissioner said 2,500–3,000 samples were taken with no breakdown given.
- **Case counts:** 104 children (4 Jan); 113 suspected (4 Jan); 108 suspected in-patients plus 152 total admitted (5 Jan); 133 registered (GMC, 6 Jan); 70 "active" (state health dept via NHRC, 8 Jan).
- **Deaths:** two child deaths on 5 Jan were declared unrelated, and a parent disputed this.

---

## 4. Greater Noida Delta-1, Jan 2026 (`greater_noida_delta1_2026.csv`, 6 rows)

I added this as a small, negative-test control.

- **Illness:** reported on 6–7 Jan 2026 (6–7 families); 7 people treated for vomiting and diarrhoea at a camp on 7 Jan.
- **Complaints:** received by the authority on 7 Jan. Earlier complaints from other blocks are undated.
- **Test:** the authority's samples of 7 Jan were "clean". I did not find the results of the follow-up samples.
- **Not found:** the "May 2024" Greater Noida case suggested in the brief. Searches returned only this Jan 2026 episode.

---

## Candidates considered and dropped
- **Pallavaram/Tambaram (Chennai), Dec 2024:** no pre-outbreak complaints or test results found. Residents say they were unaware of contamination (DT Next, opened).
- **Chitradurga Kavadigarahatti, Jul–Aug 2023:** no pre-outbreak complaints found (search only).
- **Mhow (Indore district), Jan 2026:** complaints are vague ("past few days", "for months" per Congress). Search only, not built.

---

## Context sources (verified)

### A. New Indian Express, 9 Mar 2026
URL: https://www.newindianexpress.com/india/2026/Mar/09/161-lakh-water-sample-found-contaminated-across-the-country-in-2025-26-minister-somanna-informs-rs

The article says:
- "In 2025-26, a total of 63.96 lakh samples were tested, of which 1.61 lakh were confirmed to be contaminated."
- More than 59 lakh samples were tested in 2023-24 and over 82 lakh in 2024-25.
- The figures are "as of March 2, 2026".

### B. Original Rajya Sabha answer (found)
- Rajya Sabha **Unstarred Question No. 1859**, "Testing of drinking water samples".
- Asked by Shri Sukhendu Sekhar Ray; answered on **09/03/2026** by MoS Jal Shakti Shri V. Somanna (Department of Drinking Water and Sanitation).
- PDF: https://sansad.in/getFile/annex/270/AU1859_1buoCE.pdf?source=pqars (found through the rsdoc.nic.in question-search API, session 270).

Annexure totals (source: JJM-IMIS; 2025-26 figures as on 02.03.2026):

| Year | Samples tested | Found contaminated | Remedial measures taken |
|---|---|---|---|
| 2023-24 | 59,09,306 | 7,22,322 | 4,77,601 |
| 2024-25 | 82,68,247 | 3,39,548 | 2,11,669 |
| 2025-26 | 63,96,035 | 1,61,062 | 42,384 |

- Madhya Pradesh rows: 2023-24, 5,37,146 tested / 2,553 contaminated; 2024-25, 5,74,798 / 818; 2025-26, 4,31,936 / 491.
- **Caveat:** these are **Jal Jeevan Mission (rural piped-water) WQMIS data**. They do not cover urban municipal supply such as Indore's.
- 1,61,062 / 63,96,035 = 2.52%. The Tribune (10 Mar 2026) prints 2.51%.

### C. CAG report on Bhopal/Indore water supply
- It is **Report No. 3 of the year 2019**: CAG Report on General and Social Sectors, Government of Madhya Pradesh, for the year ended 31 March 2018.
- Chapter II is the Performance Audit on Water Supply Management in Bhopal and Indore MCs, covering April 2013 to March 2018.
- The press brief says it was **presented to the MP Legislature on 21 September 2020**, even though it carries the 2019 number. The `source_published` value in the CSV uses the tabling date.

Original documents:
- Chapter II PDF: https://cag.gov.in/uploads/download_audit_report/2019/(10)%20Chapter-II%20PA%20on%20WSM-05f96ba7abda563.27223354.pdf
- Press brief: https://cag.gov.in/uploads/PressRelease/PR-Press-Brief-AR-GSS-2017-18-English-05f6a34ec4b15c5-28419229.pdf
- Report landing page (from search, not opened): https://cag.gov.in/ag2/madhya-pradesh/en/audit-report/details/112353

Exact text:
- "During the period 2013-18 total 4,481 water samples (physical, chemical and bacteriological) were adverse (below BIS 10500 standard) in both MCs".
  - Breakdown in the chapter: BMC 433 bacteriological; IMC 3,074 physical + 147 chemical + 827 bacteriological = 4,048.
  - 4,048 + 433 = 4,481. This resolves Deccan Chronicle's apparent mismatch: it attributed the IMC breakdown to both MCs.
- "Independent water sample testing was conducted jointly and it revealed that out of 54 water samples, 10 water samples were found adverse having turbidity and faecal coliform. As a result, 8.95 lakh residents (3.62 lakh in BMC and 5.33 lakh in IMC) were supplied contaminated water." The chapter's wording is "were likely to be affected".
- "The Public Health department also reported 5.45 lakh cases of water borne diseases".
- The joint sampling was done in **August–September 2018**. In IMC, 5 of 24 samples had faecal coliform at 40–140 count against a norm of zero.

**Note on Down To Earth (2 Jan 2026):** it renders 3.62/5.33 lakh as "families". The CAG says "residents", so DTE's wording is wrong.

The URL https://cag.gov.in/ag2/madhya-pradesh/en/audit-report/download/121767, linked from The Wire, is a *different* report: CAG Report No. 9 of 2024 on Local Bodies (period ending March 2022). I downloaded it but did not use it.

---

## Every URL opened

Status codes are from curl unless marked otherwise. "used" means the URL is cited in a CSV row or in the context section.

### Indore
- ✗ https://www.ndtv.com/india-news/1-dead-over-60-fall-ill-after-drinking-contaminated-water-in-cleanest-city-indore-10103758 (403 Akamai; WebFetch blocked) **FAILED**
- ✗ http://web.archive.org/web/20260104204122/https://www.ndtv.com/… (connection reset; WebFetch blocked) **FAILED**
- ✓ https://www.bhaskarenglish.in/local/mp/news/indore-water-contamination-complaints-failed-to-trigger-action-appeals-made-mayor-helpline-cm-before-deaths-136854510.html (WebFetch blocked; curl 200) — used
- ✓ https://timesofindia.indiatimes.com/city/delhi/indore-water-tragedy-fallout-delhi-jal-board-ordered-to-intensify-quality-checks-dedicated-teams-to-monitor-high-density-areas/articleshow/126387877.cms (curl 200)
  - Opened; not cited in any CSV row.
  - Says "at least 16 people died", pub 7 Jan 2026.
  - DJB directed to respond to complaints on "water quality, odour, taste, or discoloration".
- ✓ https://www.deccanherald.com/india/madhya-pradesh/3-dead-over-100-hospitalised-after-drinking-contaminated-water-in-indore-3847112 — used
- ✓ https://www.deccanherald.com/amp/story/india%2Fmadhya-pradesh%2F3-dead-over-100-hospitalised-after-drinking-contaminated-water-in-indore-3847112 (checked for the Dec-25 claim; not present)
- ✓ https://www.deccanherald.com/india/madhya-pradesh/water-contamination-in-cleanest-city-indore-death-toll-mounts-as-man-says-infant-son-among-victims-3848308 — used
- ✓ https://www.deccanherald.com/india/madhya-pradesh/leakage-in-water-pipeline-under-toilet-in-indores-bhagirathpura-led-to-water-contamination-says-lab-test-3848852 — used
- ✓ https://www.deccanherald.com/india/madhya-pradesh/indore-water-contamination-10-deaths-so-far-confirms-city-mayor-over-270-hospitalised-3849246 — used
- ✓ https://www.deccanherald.com/india/madhya-pradesh/indore-water-contamination-congress-seeks-judicial-probe-vijayvargiyas-sacking-case-against-mayor-3851395 — used
- ✓ https://www.deccanherald.com/india/madhya-pradesh/indore-water-contamination-38-new-diarrhoea-cases-detected-110-in-hospital-death-toll-7-3852434 — used
- ✓ https://www.deccanherald.com/india/madhya-pradesh/indore-water-contamination-12-new-cases-of-diarrhoea-detected-over-4800-people-screened-3859863 — used
- ✓ https://www.deccanherald.com/india/madhya-pradesh/amid-contamination-woes-indore-mayor-drinks-tap-water-in-diarrhoea-hit-bhagirathpura-3864667 — used
- ✓ https://www.deccanherald.com/india/madhya-pradesh/rahul-gandhi-meets-patients-families-of-indore-water-contamination-victims-3865353 (24 deaths claimed; 21 families compensated)
- ✓ https://www.newsonair.gov.in/five-dead-over-35-hospitalised-after-consuming-contaminated-water-in-indores-bhagirathpura — used
- ✓ https://www.newsonair.gov.in/madhya-pradesh-hc-sets-up-commission-of-inquiry-to-investigate-water-contamination-issue-in-bhagirathpura/
- ✓ https://www.newsonair.gov.in/madhya-pradesh-cm-mohan-yadav-reviews-action-over-indore-water-contamination-incident
- ✓ https://www.etvbharat.com/en/state/indore-bhagirathpura-mgm-medical-college-report-more-than-half-samples-fail-deadly-bacteria-found-enn26010805513 — used
- ✓ https://www.etvbharat.com/en/state/lab-report-confirms-contaminated-water-behind-fatal-diarrhoea-outbreak-in-madhya-pradesh-indore-bhagirathpura-enn26010200917
- ✓ https://www.etvbharat.com/en/bharat/600-page-report-unravels-bhagirathpura-deaths-in-indore-enn26090102772 (24 of 36 linked, per lawyers)
- ✓ https://www.etvbharat.com/hi/state/indore-bhagirathpura-contaminated-water-woman-death-hospitalized-ventilator-total-figure-32-deaths-govt-accept-23-madhya-pradesh-news-mps26020201520 — used
- ✓ https://www.thehansindia.com/news/national/indore-crisis-bhagirathpura-water-safe-for-pathogens-minister-vijayvargiya-shares-lab-report-1036386 — used
- ✓ https://www.ind24.tv/madhya-pradesh/bhagirathpura-water-contamination-indore-inquiry-report-36-deaths — used
- ✓ https://fvbb.com/ns/contaminated-water-in-indore-s-bhagirathpura/322337 (aggregator of an Economic Times/PTI story; ThePrint copy used instead)
- ✓ https://www.downtoearth.org.in/water/contaminated-drinking-water-case-cag-report-found-serious-flaws-but-indore-remained-unconcerned
- ✓ https://www.groundreport.in/states-madhya-pradesh/what-is-indores-bhagirathpura-contaminated-water-tragedy/ — used
- ✓ https://gulfnews.com/world/asia/india/indore-water-horror-police-outpost-toilet-withoutseptic-tank-linked-to-10-deaths-1.500398092
- ✓ https://m.thewire.in/article/government/indore-water-contamination-deaths-judicial-commission-says-tragedy-was-preventable — used
- ✓ https://www.dtnext.in/news/national/indore-water-linked-deaths-preventable-pipeline-tender-delays-contributed-to-tragedy-judicial-panel — used
- ✓ https://thelogicalindian.com/indore-deaths-to-sewage-contaminated-water-124970/
- ✓ https://navabharat.com/?p=584234 (downloaded; not analysed)
- ✓ https://theprint.in/india/bacterial-contamination-of-drinking-water-led-to-indore-tragedy-mp-govt-to-hc/2832316 — used
- ✓ https://theprint.in/india/contaminated-water-in-indores-bhagirathpura-result-of-decades-of-systemic-failure-probe-commission/3058622/ — used
- ✓ https://theprint.in/india/mp-water-contamination-ngt-constitutes-6-member-panel-to-probe-issue/2828322
- ✓ https://theprint.in/india/indore-water-contamination-mp-hc-questions-death-audit-appoints-retired-judge-for-judicial-probe/2838655 — used
- ✓ https://theprint.in/india/odour-from-taps-flood-of-complaints-indore-residents-had-pointed-to-foul-water-but-no-one-acted/2816985/ — used
- ✓ https://www.freepressjournal.in/bhopal/national-green-tribunal-seeks-accountability-over-indore-water-tragedy (MPPCB 50/52, 40/52)
- ✓ https://www.freepressjournal.in/indore/indore-horror-3-dead-100-ill-after-pipeline-leak-mixes-sewer-water-into-drinking-supply-families-say-complaints-ignored-for-10-days — used
- ✓ https://www.freepressjournal.in/indore/mp-news-bhagirathpura-turns-into-a-death-zone — used
- ✓ https://www.freepressjournal.in/indore/indore-water-deaths-water-is-so-harmful-that-my-dads-both-kidneys-are-damaged-bhagirathpura-resident-shares-ordeal-after-hospital-hands-him-15-lakh-bill-video — used
- ✓ https://www.freepressjournal.in/indore/indore-news-bhagirathpura-death-toll-rises-to-27-with-two-more-deaths — used
- ✓ https://www.nationalheraldindia.com/politics/congress-claims-98-percent-of-indore-water-samples-contaminated-seeks-city-wide-audit (downloaded; not used)
- ✓ https://www.thelallantop.com/india/post/indore-bhagirathpura-complaints-not-addressed-bureaucracy-delay-pipeline-repair — used
- ✓ https://www.republicbharat.com/india/madhya-pradesh/indore-people-have-been-drinking-dirty-water-for-three-years-over-400-complaints-were-received-in-december — used
- ✓ https://www.republicworld.com/india/complained-for-days-but-no-one-listened-fear-grips-indore-as-killer-water-claims-lives
- ✓ https://english.dainikjagranmpcg.com/states/madhya-pradesh/bhagirathpura-tragedy-raises-accountability-questions-after-20-die-from-contaminated/article-12161 — used
- ✓ https://aninews.in/news/national/general-news/mp-locals-fear-using-tanker-water-after-contaminated-water-incident-in-indores-bhagirathpura20260102104819/ — used
- ✓ https://www.aajtak.in/madhya-pradesh/story/indore-bhagirathpura-contaminated-water-illness-deaths-lclar-strc-2426693-2025-12-30 — used
- ✓ https://www.millenniumpost.in/nation/officials-ignored-dirty-water-complaints-say-victims-families-643665 — used
- ✓ https://www.millenniumpost.in/nation/reports-of-26-samples-out-of-50-indicates-presence-of-bacteria-says-imc-additional-commissioner-on-indore-water-contamination-issue-642364 — used
- ✓ https://www.deccanchronicle.com/nation/mp-indore-tragedy-lab-tests-of-water-samples-suggest-faecal-coliform-contamination-1928584 — used
- ✓ https://www.deccanchronicle.com/nation/pcb-survey-confirms-faecal-contamination-in-indores-groundwater-1929670 — used
- ✓ https://www.siasat.com/indore-water-contamination-hc-orders-safekeep-of-original-pipeline-tender-3329414/ (corroborates 51 tube wells / E. coli; status report of 15 Jan)
- ✓ https://m.thewire.in/article/government/10-dead-2000-fall-sick-in-indore-after-leakage-in-municipal-pipeline-leads-to-drinking-water-contamination (mobile copy; desktop thewire.in unreachable as of 2026-11-05)
- ✓ https://madhyamamonline.com/india/contaminated-water-behind-indore-health-crisis-lab-findings-status-report-in-hc-tomorrow-1482296 (MGM report submitted Thu 1 Jan; 272/72/201/32)
- ✓ https://thelivenagpur.com/2026/01/02/ignored-warnings-delayed-repairs-how-negligence-led-to-deadly-water-contamination-in-indores-bhagirathpura/ — used
- ✓ https://en.wikipedia.org/wiki/2025_Indore_drinking_water_contamination (background only; not cited)
- ✓ https://inshorts.com/prev/en/news/what-is-kerala-government-approved--nativity-card---1767344167343 (irrelevant page; not used)
- ✗ https://mphc.gov.in/upload/indore/MPHCIND/2025/WP/50641/WP_50641_2025_Order_06-01-2026.pdf (curl connection reset ×3; WebFetch 503) **FAILED**
- ✗ https://www.newsbytesapp.com/news/india/indore-water-tragedy-mayor-s-helpline-got-warning-but-none-listened/story (403) **FAILED**

### Shimla
- ✓ https://www.tribuneindia.com/news/archive/community/jaundice-cases-mount-to-500-in-shimla-180306/ — used
- ✓ https://www.tribuneindia.com/news/archive/features/govt-apathy-led-to-jaundice-outbreak-says-niv-report-197674 — used
- ✓ https://www.tribuneindia.com/news/archive/features/two-junior-engineers-arrested-192508 — used
- ✓ https://www.tribuneindia.com/news/archive/features/jaundice-threat-looms-due-to-misplaced-stps-203769 — used
- ✓ https://www.tribuneindia.com/news/archive/jaundice-cases-declining-says-chief-secy-190722 (not used)
- ✓ https://www.tribuneindia.com/news/archive/features/jaundice-deaths-spark-protest-in-shimla-200197 — used
- ✓ https://www.tribuneindia.com/news/archive/dushyant-bhatt-is-panipat-senior-dy-mayor-200847 ("Jaundice has claimed 10 lives: Kaul") — used
- ✓ https://www.tribuneindia.com/news/archive/himachal/jaundice-claims-its-first-victim-in-state-530696/ (**2018, a different outbreak; excluded**)
- ✓ https://www.downtoearth.org.in/water/sewage-treatment-plant-failure-leads-to-jaundice-outbreak-in-shimla-52593 — used
- ✓ https://himachalwatcher.com/2016/01/02/shimla-at-high-risk-of-jaundice-outbreak-due-to-poor-quality-drinking-water-supply-report/ — used
- ✓ https://himachalwatcher.com/2016/02/12/shimla-solan-jaundice-alert-all-water-samples-tested-positive-for-hepatitis-e/ — used
- ✓ https://himachalwatcher.com/2016/02/20/shimla-jaundice-outbreak-3-deaths-alone-in-last-60-hours-total-count-reaches-10/ (corroborates 10 deaths; first case Dec 2015)
- ✓ https://thenewshimachal.com/2016/01/health-minister-reviews-steps-to-tackle-jaundice-in-shimla-town/ — used
- ✓ https://www.ijcmph.com/index.php/ijcmph/article/download/1263/1183/5461 — used
- ✓ https://www.worldwidejournals.com/indian-journal-of-applied-research-(IJAR)/recent_issues_pdf/2018/February/February_2018_1517474358__84.pdf — used
- ✓ https://www.worldwidejournals.com/indian-journal-of-applied-research-(IJAR)/article/early-detection-of-hepatitis-e-a-outbreak-in-shimla-…/MTQ2Mjk=/?is=1 (abstract page)
- ✓ https://en.wikipedia.org/wiki/Shimla_jaundice_outbreak (background only)
- ✗ https://www.medindia.net/news/jaundice-deaths-haunt-shimla-himachal-pradesh-157901-1.htm (403) **FAILED**

### Gandhinagar
- ✓ https://theprint.in/india/gandhinagar-typhoid-outbreak-tally-of-suspected-cases-rises-to-108-after-25-new-additions/2819500/ — used
- ✓ https://theprint.in/india/outbreak-of-typhoid-in-gandhinagar-hm-amit-shah-directs-admin-to-act-on-a-war-footing/2818909 (same ANI copy as below)
- ✓ https://www.deccanherald.com/india/gujarat/gujarat-typhoid-outbreak-over-150-hospitalised-in-gandhinagar-3852339 — used
- ✓ https://www.deccanherald.com/india/gujarat/nhrc-issues-notice-to-gujarat-govt-over-typhoid-outbreak-seeks-detailed-report-in-two-weeks-3855612
- ✓ https://www.medboundtimes.com/india/gandhinagar-typhoid-outbreak-contaminated-water — used
- ✓ https://newsable.asianetnews.com/india/gandhinagar-typhoid-surge-no-need-to-panic-water-quality-checked-articleshow-24sx6ck — used
- ✓ https://www.newsonair.gov.in/hm-amit-shah-directs-state-administration-to-tackle-gandhinagar-typhoid-outbreak-on-war-footing
- ✓ https://www.newsonair.gov.in/nhrc-takes-suo-motu-cognisance-of-reported-surge-in-typhoid-cases-in-gandhinagar-gujarat — used
- ✓ https://english.bombaysamachar.com/gujarat/what-went-wrong-in-gujarats-smart-city-gandhinagar-100-children-pay-the-price-after-water-contamination/ — used
- ✓ https://navbharatlive.com/gujarat/contaminated-water-supply-reported-in-gandhinagar-104-children-hospitalized-1496932.html — used
- ✓ https://aninews.in/news/national/general-news/gujarat-amit-shah-directs-administration-to-act-on-war-footing-as-gandhinagar-faces-typhoid-outbreak20260105022301/ — used
- ✓ https://aninews.in/news/national/general-news/gujarat-gmc-issues-clarification-on-typhoid-cases-says-reports-of-deaths-incorrect-misleading20260106052534/ — used
- ✓ https://www.tribuneindia.com/news/gandhinagartyphoid/after-indore-crisis-contaminated-water-causes-typhoid-outbreak-in-gandhinagar (PTI copy of ThePrint item)
- ✓ https://m.thewire.in/article/health/gujarat-typhoid-outbreak-sewage-contaminates-drinking-water-network-over-100-cases-reported
- ✗ https://m.thewire.in/article/health/gujarat-typhoid-outbreak-sewage-contaminates-drinking-water-network-over-100-cases-reported (mobile copy loads as of 2026-11-05; desktop thewire.in unreachable; not used)
- ✗ https://www.opindia.com/news-updates/typhoid-outbreak-in-gandhinagar-due-to-contaminated-water/ (403) **FAILED**
- ✗ https://www.dnaindia.com/india/report-after-indore-contaminated-water-tragedy-hits-gujarat-s-gandhinagar-over-100-hospitalised-with-typhoid-3195937 (403) **FAILED**
- ✗ https://www.theweek.in/news/health/2026/01/06/after-indore-gandhinagar-faces-typhoid-outbreak-know-more-about-symptoms-prevention.html (403) **FAILED**

### Greater Noida and other candidates
- ✓ https://www.deccanherald.com/india/uttar-pradesh/several-fall-ill-in-greater-noida-after-drinking-contaminated-water-authority-denies-sewage-mixing-3855173 — used
- ✓ https://www.tribuneindia.com/news/delhi/residents-fall-ill-due-to-contaminated-tap-water-in-gr-noida/ — used (as deaths_source in the YAML)
- ✓ https://www.nationalheraldindia.com/national/residents-fall-ill-in-greater-noidas-delta-1-amid-fears-of-contaminated-water (corroborating)
- ✓ https://www.dtnext.in/news/chennai/pallavaram-water-contamination-an-avoidable-tragedy-had-tambaram-corpn-done-its-duty-say-residents-813992 (Pallavaram; dropped)

### Context
- ✓ https://www.newindianexpress.com/india/2026/Mar/09/161-lakh-water-sample-found-contaminated-across-the-country-in-2025-26-minister-somanna-informs-rs
- ✓ https://rsdoc.nic.in/Question/Search_Questions?whereclause=… (API lookup → AU1859)
- ✓ https://sansad.in/getFile/annex/270/AU1859_1buoCE.pdf?source=pqars
- ✓ https://sansad.in/rs/questions/questions-and-answers (landing page only)
- ✓ https://www.tribuneindia.com/news/india/63-96-lakh-water-samples-tested-in-1-year-1-6-lakh-found-contaminated/
- ✓ https://www.etvbharat.com/en/bharat/several-water-samples-found-contaminated-in-2025-26-govt-in-rajya-sabha-enn26030905891
- ✓ https://cag.gov.in/uploads/download_audit_report/2019/(10)%20Chapter-II%20PA%20on%20WSM-05f96ba7abda563.27223354.pdf
- ✓ https://cag.gov.in/uploads/PressRelease/PR-Press-Brief-AR-GSS-2017-18-English-05f6a34ec4b15c5-28419229.pdf
- ✓ https://cag.gov.in/ag2/madhya-pradesh/en/audit-report/download/121767 (Report No. 9 of 2024; not the 2019 report)
- ✓ https://m.thewire.in/article/environment/indore-water-contamination-cag-flagged-water-borne-diseases-lack-of-water-testing-adverse-impact-on-population
- ✓ https://www.deccanchronicle.com/nation/cag-warned-of-water-contamination-and-disease-outbreak-in-indore-and-bhopal-6-years-ago-1928161
- ✗ https://www.stocklens.co.in/mp/sukhendu-sekhar-ray (403) **FAILED**
