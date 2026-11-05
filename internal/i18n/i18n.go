// Package i18n holds every resident-facing message in English, Hindi,
// Marathi and Telugu. Messages are plain templates with {name} placeholders.
//
// Safety: advisory and consent wording must be reviewed by a doctor/public-
// health professional and a native speaker before a real pilot sends them;
// the reviewer is recorded in BHAGIRATH_ADVISORY_REVIEWED_BY and shown in the
// README. Bhagirath never claims to be a government body.
package i18n

import (
	"sort"
	"strings"
)

// Vars are template variables.
type Vars map[string]string

// T renders message key in language l, falling back to Hindi then English.
func T(key, l string, v Vars) string {
	m, ok := messages[key]
	if !ok {
		return key
	}
	s, ok := m[l]
	if !ok {
		if s, ok = m["hi"]; !ok || l == "en" {
			s = m["en"]
		}
	}
	if len(v) == 0 {
		return s
	}
	keys := make([]string, 0, len(v))
	for k := range v {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	pairs := make([]string, 0, 2*len(v))
	for _, k := range keys {
		pairs = append(pairs, "{"+k+"}", v[k])
	}
	return strings.NewReplacer(pairs...).Replace(s)
}

// Has reports whether key exists in language l (no fallback).
func Has(key, l string) bool {
	_, ok := messages[key][l]
	return ok
}

// Keys lists all message keys (for completeness tests).
func Keys() []string {
	out := make([]string, 0, len(messages))
	for k := range messages {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

var messages = map[string]map[string]string{
	"consent_notice": {
		"en": "Bhagirath (with {partner}): we received your water complaint. To check later that it was really fixed, we would like to call or message you back. Your number stays private and is never shown to officials. Reply YES to allow callbacks or NO to refuse. Bhagirath is a citizen tool, not a government service.",
		"hi": "भागीरथ ({partner} के साथ): आपकी पानी की शिकायत मिल गई है। बाद में यह जाँचने के लिए कि समस्या सच में ठीक हुई या नहीं, हम आपको कॉल या संदेश करना चाहते हैं। आपका नंबर गोपनीय रहेगा और अधिकारियों को नहीं दिखाया जाएगा। कॉल-बैक की अनुमति के लिए YES (हाँ) लिखें, मना करने के लिए NO (नहीं)। भागीरथ नागरिकों का साधन है, सरकारी सेवा नहीं।",
		"mr": "भगीरथ ({partner} सोबत): तुमची पाण्याची तक्रार मिळाली आहे. समस्या खरोखर दूर झाली का हे नंतर तपासण्यासाठी आम्ही तुम्हाला कॉल किंवा संदेश करू इच्छितो. तुमचा नंबर गोपनीय राहील आणि अधिकाऱ्यांना दाखवला जाणार नाही. परवानगीसाठी YES (हो) लिहा, नकारासाठी NO (नाही). भगीरथ हे नागरिकांचे साधन आहे, सरकारी सेवा नाही.",
		"te": "భగీరథ ({partner} తో కలిసి): మీ నీటి ఫిర్యాదు అందింది. సమస్య నిజంగా పరిష్కారమైందో లేదో తర్వాత తెలుసుకోవడానికి మేము మీకు కాల్ లేదా సందేశం చేయాలనుకుంటున్నాం. మీ నంబర్ గోప్యంగా ఉంటుంది, అధికారులకు చూపించబడదు. అనుమతికి YES (అవును), వద్దనుకుంటే NO (కాదు) అని పంపండి. భగీరథ పౌరుల సాధనం, ప్రభుత్వ సేవ కాదు.",
	},
	"consent_thanks": {
		"en": "Thank you. We will call you back only about this water complaint.",
		"hi": "धन्यवाद। हम आपको सिर्फ़ इसी पानी की शिकायत के बारे में कॉल करेंगे।",
		"mr": "धन्यवाद. आम्ही फक्त याच पाण्याच्या तक्रारीबद्दल तुम्हाला कॉल करू.",
		"te": "ధన్యవాదాలు. ఈ నీటి ఫిర్యాదు గురించి మాత్రమే మేము మిమ్మల్ని సంప్రదిస్తాం.",
	},
	"consent_declined": {
		"en": "Understood. Your complaint is recorded without your number and we will not contact you.",
		"hi": "ठीक है। आपकी शिकायत आपके नंबर के बिना दर्ज है, हम आपसे संपर्क नहीं करेंगे।",
		"mr": "ठीक आहे. तुमची तक्रार तुमच्या नंबरशिवाय नोंदवली आहे, आम्ही तुमच्याशी संपर्क करणार नाही.",
		"te": "సరే. మీ ఫిర్యాదు మీ నంబర్ లేకుండా నమోదైంది, మేము మిమ్మల్ని సంప్రదించం.",
	},
	"complaint_received": {
		"en": "Bhagirath: complaint {ref} recorded — {issue}. We will send it to the responsible officials and follow up until it is fixed.",
		"hi": "भागीरथ: शिकायत {ref} दर्ज हो गई — {issue}। हम इसे ज़िम्मेदार अधिकारियों तक भेजेंगे और ठीक होने तक पीछा करेंगे।",
		"mr": "भगीरथ: तक्रार {ref} नोंदवली — {issue}. आम्ही ती जबाबदार अधिकाऱ्यांकडे पाठवू आणि दुरुस्त होईपर्यंत पाठपुरावा करू.",
		"te": "భగీరథ: ఫిర్యాదు {ref} నమోదైంది — {issue}. దీన్ని బాధ్యులైన అధికారులకు పంపి, పరిష్కారమయ్యే వరకు అనుసరిస్తాం.",
	},
	"ask_location": {
		"en": "To send complaint {ref} to the right ward, please share your location pin, or reply with a nearby landmark (temple, school, shop, water tank).",
		"hi": "शिकायत {ref} सही वार्ड तक भेजने के लिए कृपया अपनी लोकेशन पिन भेजें, या पास की कोई जगह बताएँ (मंदिर, स्कूल, दुकान, पानी की टंकी)।",
		"mr": "तक्रार {ref} योग्य वॉर्डकडे पाठवण्यासाठी कृपया तुमचे लोकेशन पिन पाठवा, किंवा जवळचे ठिकाण सांगा (मंदिर, शाळा, दुकान, पाण्याची टाकी).",
		"te": "ఫిర్యాదు {ref} ను సరైన వార్డుకు పంపడానికి దయచేసి మీ లొకేషన్ పిన్ పంపండి, లేదా దగ్గర్లోని గుర్తు చెప్పండి (గుడి, బడి, దుకాణం, నీటి ట్యాంక్).",
	},
	"location_thanks": {
		"en": "Thank you — complaint {ref} is now linked to {ward}.",
		"hi": "धन्यवाद — शिकायत {ref} अब {ward} से जुड़ गई है।",
		"mr": "धन्यवाद — तक्रार {ref} आता {ward} शी जोडली आहे.",
		"te": "ధన్యవాదాలు — ఫిర్యాదు {ref} ఇప్పుడు {ward} కి జోడించబడింది.",
	},
	"callback_question": {
		"en": "Bhagirath: an official says the water problem in your complaint {ref} has been fixed. Is it really fixed at your home? Reply 1 for YES or 2 for NO.",
		"hi": "भागीरथ: एक अधिकारी का कहना है कि आपकी शिकायत {ref} वाली पानी की समस्या ठीक हो गई है। क्या आपके घर पर सच में ठीक हो गई है? हाँ के लिए 1, नहीं के लिए 2 भेजें।",
		"mr": "भगीरथ: एका अधिकाऱ्याच्या मते तुमच्या तक्रार {ref} मधील पाण्याची समस्या दूर झाली आहे. तुमच्या घरी खरोखर दूर झाली आहे का? हो साठी 1, नाही साठी 2 पाठवा.",
		"te": "భగీరథ: మీ ఫిర్యాదు {ref} లోని నీటి సమస్య పరిష్కారమైందని ఒక అధికారి చెబుతున్నారు. మీ ఇంట్లో నిజంగా పరిష్కారమైందా? అవును అయితే 1, కాకపోతే 2 పంపండి.",
	},
	"callback_yes": {
		"en": "Thank you for confirming. Complaint {ref} is now marked as fixed and verified.",
		"hi": "पुष्टि के लिए धन्यवाद। शिकायत {ref} अब ठीक और सत्यापित मानी गई है।",
		"mr": "पुष्टी केल्याबद्दल धन्यवाद. तक्रार {ref} आता दुरुस्त व सत्यापित म्हणून नोंदवली आहे.",
		"te": "నిర్ధారించినందుకు ధన్యవాదాలు. ఫిర్యాదు {ref} ఇప్పుడు పరిష్కారమై, ధృవీకరించబడింది.",
	},
	"callback_no": {
		"en": "Thank you. Complaint {ref} has been reopened and escalated to a more senior official.",
		"hi": "धन्यवाद। शिकायत {ref} दोबारा खोली गई है और वरिष्ठ अधिकारी तक भेजी गई है।",
		"mr": "धन्यवाद. तक्रार {ref} पुन्हा उघडली असून वरिष्ठ अधिकाऱ्यांकडे पाठवली आहे.",
		"te": "ధన్యవాదాలు. ఫిర్యాదు {ref} మళ్లీ తెరవబడింది, పై అధికారికి పంపబడింది.",
	},
	"advisory": {
		"en": "PRECAUTIONARY SAFETY MESSAGE — {ward}. Signs of possible drinking-water contamination have been reported in your area. Until further notice: 1) Boil drinking water at a rolling boil for at least 1 minute, or use a known safe source. 2) For diarrhoea, give ORS and keep drinking fluids. 3) Get medical care at once for blood in stool, not able to drink, being very sleepy, or passing very little urine. 4) To report a problem: {how}. This is a precautionary safety message from Bhagirath and {partner}. It is not an official government order.",
		"hi": "एहतियाती सुरक्षा संदेश — {ward}। आपके क्षेत्र में पीने का पानी दूषित होने के संकेत मिले हैं। अगली सूचना तक: 1) पीने के पानी को कम से कम 1 मिनट तक खौलाकर उबालें, या किसी भरोसेमंद सुरक्षित स्रोत का पानी पिएँ। 2) दस्त होने पर ORS घोल दें और तरल पदार्थ पिलाते रहें। 3) मल में खून, पानी न पी पाना, बहुत ज़्यादा सुस्ती, या बहुत कम पेशाब होने पर तुरंत डॉक्टर को दिखाएँ। 4) समस्या बताने के लिए: {how}। यह भागीरथ और {partner} की ओर से एहतियाती सुरक्षा संदेश है। यह कोई सरकारी आदेश नहीं है।",
		"mr": "खबरदारीचा सुरक्षा संदेश — {ward}. तुमच्या भागात पिण्याचे पाणी दूषित असल्याची लक्षणे आढळली आहेत. पुढील सूचनेपर्यंत: 1) पिण्याचे पाणी किमान 1 मिनिट खळखळून उकळा, किंवा खात्रीच्या सुरक्षित स्रोताचे पाणी वापरा. 2) जुलाब झाल्यास ORS द्या आणि पातळ पदार्थ देत राहा. 3) शौचात रक्त, पाणी पिता न येणे, खूप सुस्ती किंवा खूप कमी लघवी झाल्यास लगेच डॉक्टरकडे जा. 4) समस्या कळवण्यासाठी: {how}. हा भगीरथ आणि {partner} यांच्याकडून खबरदारीचा सुरक्षा संदेश आहे. हा सरकारी आदेश नाही.",
		"te": "ముందుజాగ్రత్త భద్రతా సందేశం — {ward}. మీ ప్రాంతంలో తాగునీరు కలుషితమైనట్లు సూచనలు వచ్చాయి. తదుపరి సూచన వరకు: 1) తాగునీటిని కనీసం 1 నిమిషం బాగా మరిగించండి, లేదా నమ్మకమైన సురక్షిత నీటిని వాడండి. 2) విరేచనాలు అయితే ORS ఇవ్వండి, ద్రవాలు తాగిస్తూ ఉండండి. 3) మలంలో రక్తం, తాగలేకపోవడం, చాలా మగతగా ఉండటం, లేదా మూత్రం చాలా తక్కువగా రావడం జరిగితే వెంటనే వైద్యుడిని సంప్రదించండి. 4) సమస్య తెలియజేయడానికి: {how}. ఇది భగీరథ మరియు {partner} నుండి ముందుజాగ్రత్త భద్రతా సందేశం. ఇది ప్రభుత్వ అధికారిక ఉత్తర్వు కాదు.",
	},
	"how_to_report": {
		"en": "give a missed call or send a WhatsApp voice note to this number",
		"hi": "इस नंबर पर मिस्ड कॉल दें या व्हाट्सऐप पर वॉइस नोट भेजें",
		"mr": "या नंबरवर मिस्ड कॉल द्या किंवा व्हॉट्सॲपवर व्हॉइस नोट पाठवा",
		"te": "ఈ నంబర్\u200cకు మిస్డ్ కాల్ ఇవ్వండి లేదా వాట్సాప్\u200cలో వాయిస్ నోట్ పంపండి",
	},
	"illness_ack": {
		"en": "Bhagirath: illness report {ref} recorded for {ward} — {summary}. Thank you. (Send counts only; never patient names.)",
		"hi": "भागीरथ: {ward} के लिए बीमारी रिपोर्ट {ref} दर्ज — {summary}। धन्यवाद। (सिर्फ़ संख्या भेजें, मरीज़ों के नाम कभी नहीं।)",
		"mr": "भगीरथ: {ward} साठी आजार अहवाल {ref} नोंदवला — {summary}. धन्यवाद. (फक्त संख्या पाठवा; रुग्णांची नावे कधीही नाही.)",
		"te": "భగీరథ: {ward} కోసం అనారోగ్య నివేదిక {ref} నమోదైంది — {summary}. ధన్యవాదాలు. (సంఖ్యలు మాత్రమే పంపండి; రోగుల పేర్లు ఎప్పుడూ వద్దు.)",
	},
	"illness_format_help": {
		"en": "Bhagirath: please send daily counts like: D 3 V 1 ORS 12 (D = diarrhoea cases, V = vomiting cases, ORS = packets sold). Counts only, no names.",
		"hi": "भागीरथ: कृपया रोज़ की संख्या ऐसे भेजें: D 3 V 1 ORS 12 (D = दस्त के मरीज़, V = उल्टी के मरीज़, ORS = बिके पैकेट)। सिर्फ़ संख्या, नाम नहीं।",
		"mr": "भगीरथ: कृपया रोजची संख्या अशी पाठवा: D 3 V 1 ORS 12 (D = जुलाबाचे रुग्ण, V = उलटीचे रुग्ण, ORS = विकलेली पाकिटे). फक्त संख्या, नावे नाहीत.",
		"te": "భగీరథ: దయచేసి రోజువారీ సంఖ్యలను ఇలా పంపండి: D 3 V 1 ORS 12 (D = విరేచనాల కేసులు, V = వాంతుల కేసులు, ORS = అమ్మిన ప్యాకెట్లు). సంఖ్యలు మాత్రమే, పేర్లు వద్దు.",
	},
	"ivr_welcome": {
		"en": "Welcome to Bhagirath, a free citizen service for reporting dirty drinking water. This is not a government line.",
		"hi": "भागीरथ में आपका स्वागत है। यह गंदे पीने के पानी की शिकायत के लिए मुफ़्त नागरिक सेवा है। यह सरकारी लाइन नहीं है।",
		"mr": "भगीरथमध्ये आपले स्वागत आहे. ही दूषित पिण्याच्या पाण्याची तक्रार करण्यासाठी मोफत नागरिक सेवा आहे. ही सरकारी लाइन नाही.",
		"te": "భగీరథకు స్వాగతం. మురికి తాగునీటి గురించి ఫిర్యాదు చేయడానికి ఇది ఉచిత పౌర సేవ. ఇది ప్రభుత్వ లైన్ కాదు.",
	},
	"ivr_consent": {
		"en": "We may call you back later to check that the problem was fixed. Your number stays private. Press 1 to allow this, or press 2 to continue without callbacks.",
		"hi": "समस्या ठीक हुई या नहीं, यह जाँचने के लिए हम बाद में आपको कॉल कर सकते हैं। आपका नंबर गोपनीय रहेगा। अनुमति देने के लिए 1 दबाएँ, बिना कॉल-बैक के आगे बढ़ने के लिए 2 दबाएँ।",
		"mr": "समस्या दूर झाली का हे तपासण्यासाठी आम्ही नंतर तुम्हाला कॉल करू शकतो. तुमचा नंबर गोपनीय राहील. परवानगीसाठी 1 दाबा, कॉल-बॅकशिवाय पुढे जाण्यासाठी 2 दाबा.",
		"te": "సమస్య పరిష్కారమైందో లేదో తెలుసుకోవడానికి తర్వాత మీకు కాల్ చేయవచ్చు. మీ నంబర్ గోప్యంగా ఉంటుంది. అనుమతికి 1 నొక్కండి, కాల్-బ్యాక్ లేకుండా కొనసాగడానికి 2 నొక్కండి.",
	},
	"ivr_record": {
		"en": "After the beep, please describe the water problem and say a landmark near your home, like a temple, school or shop. Press hash when done.",
		"hi": "बीप के बाद कृपया पानी की समस्या बताइए और अपने घर के पास की कोई जगह बताइए, जैसे मंदिर, स्कूल या दुकान। बोलने के बाद हैश दबाएँ।",
		"mr": "बीपनंतर कृपया पाण्याची समस्या सांगा आणि घराजवळचे एखादे ठिकाण सांगा, जसे मंदिर, शाळा किंवा दुकान. झाल्यावर हॅश दाबा.",
		"te": "బీప్ తర్వాత దయచేసి నీటి సమస్య చెప్పండి, మీ ఇంటి దగ్గర ఉన్న గుడి, బడి లేదా దుకాణం లాంటి గుర్తు చెప్పండి. పూర్తయ్యాక హాష్ నొక్కండి.",
	},
	"ivr_thanks": {
		"en": "Thank you. Your complaint has been recorded. Goodbye.",
		"hi": "धन्यवाद। आपकी शिकायत दर्ज हो गई है। नमस्ते।",
		"mr": "धन्यवाद. तुमची तक्रार नोंदवली आहे. नमस्कार.",
		"te": "ధన్యవాదాలు. మీ ఫిర్యాదు నమోదైంది. నమస్కారం.",
	},
	"ivr_callback": {
		"en": "This is Bhagirath, calling about your water complaint. An official says the problem has been fixed. If the water at your home is now fine, press 1. If the problem is still there, press 2.",
		"hi": "यह भागीरथ की ओर से आपकी पानी की शिकायत के बारे में कॉल है। एक अधिकारी का कहना है कि समस्या ठीक हो गई है। अगर अब आपके घर का पानी ठीक है तो 1 दबाएँ। अगर समस्या अब भी है तो 2 दबाएँ।",
		"mr": "हा भगीरथकडून तुमच्या पाण्याच्या तक्रारीबद्दल कॉल आहे. एका अधिकाऱ्याच्या मते समस्या दूर झाली आहे. तुमच्या घरचे पाणी आता ठीक असेल तर 1 दाबा. समस्या अजूनही असेल तर 2 दाबा.",
		"te": "ఇది మీ నీటి ఫిర్యాదు గురించి భగీరథ నుండి కాల్. సమస్య పరిష్కారమైందని ఒక అధికారి చెబుతున్నారు. మీ ఇంట్లో నీరు ఇప్పుడు బాగుంటే 1 నొక్కండి. సమస్య ఇంకా ఉంటే 2 నొక్కండి.",
	},
	"ivr_advisory_intro": {
		"en": "This is a precautionary safety message from Bhagirath.",
		"hi": "यह भागीरथ की ओर से एहतियाती सुरक्षा संदेश है।",
		"mr": "हा भगीरथकडून खबरदारीचा सुरक्षा संदेश आहे.",
		"te": "ఇది భగీరథ నుండి ముందుజాగ్రత్త భద్రతా సందేశం.",
	},
	"issue_dirty_water":   {"en": "dirty water", "hi": "गंदा पानी", "mr": "गढूळ/घाण पाणी", "te": "మురికి నీరు"},
	"issue_foul_smell":    {"en": "foul-smelling water", "hi": "बदबूदार पानी", "mr": "दुर्गंधीयुक्त पाणी", "te": "దుర్వాసన నీరు"},
	"issue_sewage_mixing": {"en": "sewage mixing in water", "hi": "पानी में सीवेज/नाली का पानी", "mr": "पाण्यात सांडपाणी मिसळणे", "te": "నీటిలో మురుగు కలవడం"},
	"issue_no_water":      {"en": "no water supply", "hi": "पानी नहीं आ रहा", "mr": "पाणी येत नाही", "te": "నీళ్ళు రావడం లేదు"},
	"issue_leak":          {"en": "pipe leak", "hi": "पाइप लीकेज", "mr": "पाईप गळती", "te": "పైపు లీక్"},
	"issue_illness":       {"en": "illness after drinking water", "hi": "पानी पीने से बीमारी", "mr": "पाणी पिऊन आजार", "te": "నీరు తాగి అనారోగ్యం"},
	"issue_other":         {"en": "water problem", "hi": "पानी की समस्या", "mr": "पाण्याची समस्या", "te": "నీటి సమస్య"},
}

// Issue renders an issue type label.
func Issue(issueType, l string) string { return T("issue_"+issueType, l, nil) }

// TwilioVoice maps languages to Twilio <Say> voice/language pairs.
var TwilioVoice = map[string][2]string{
	"en": {"Polly.Raveena", "en-IN"},
	"hi": {"Polly.Aditi", "hi-IN"},
	"mr": {"Google.mr-IN-Standard-A", "mr-IN"},
	"te": {"Google.te-IN-Standard-A", "te-IN"},
}
