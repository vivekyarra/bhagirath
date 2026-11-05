package extract

import "github.com/vivekyarra/bhagirath/internal/domain"

// lexicon maps issue types to multilingual cue phrases (English, Hindi,
// romanised Hindi, Marathi, Telugu, Tamil, Bengali, Gujarati, Kannada).
// Phrases are matched against normalised text; longer phrases win ties.
// NOTE: maintained with native speakers during the pilot; additions are logged
// in docs/LEXICON.md so ASR/extraction accuracy changes stay auditable.
var lexicon = map[domain.IssueType][]string{
	domain.IssueSewageMixing: {
		"sewage", "sewer", "drain water", "drainage", "gutter", "nala", "naala", "nali ka pani", "naali", "toilet water", "latrine",
		"sewage mixed", "mixing with drain", "mixed with sewage", "chamber",
		"सीवेज", "सीवर", "नाली", "नाले", "नाला", "गटर", "ड्रेनेज", "मल", "शौचालय का पानी", "चैंबर", "नाली का पानी", "मिल रहा",
		"गटार", "सांडपाणी", "ड्रेनेजचे", "मैला",
		"మురుగు", "డ్రైనేజీ", "మురుగునీరు", "కాలువ నీరు",
		"கழிவுநீர்", "சாக்கடை",
		"নর্দমা", "ড্রেন", "নালা",
		"ગટર", "ડ્રેનેજ",
		"ಚರಂಡಿ", "ಒಳಚರಂಡಿ",
	},
	domain.IssueFoulSmell: {
		"smell", "smelly", "stink", "stinks", "stinking", "odour", "odor", "foul", "bad smell", "rotten",
		"badbu", "badboo", "bu aa", "boo aa", "durgandh", "smell aa",
		"बदबू", "बदबूदार", "दुर्गंध", "बू आ", "गंध", "सड़ी", "सड़ा",
		"दुर्गंधी", "घाण वास", "वास येत",
		"దుర్వాసన", "వాసన", "కంపు",
		"துர்நாற்றம்", "நாற்றம்",
		"দুর্গন্ধ", "গন্ধ",
		"દુર્ગંધ", "વાસ",
		"ದುರ್ವಾಸನೆ", "ವಾಸನೆ",
	},
	domain.IssueDirtyWater: {
		"dirty", "muddy", "brown water", "yellow water", "black water", "turbid", "cloudy", "contaminated", "worms", "insects",
		"keede", "keeda", "kida", "ganda", "gandā", "gandi", "matmaila", "matmela", "peela pani", "kala pani", "mitti", "kachra",
		"गंदा", "गंदी", "गन्दा", "मटमैला", "पीला पानी", "काला पानी", "कीड़े", "कीड़ा", "मिट्टी", "कचरा", "दूषित", "झाग",
		"गढूळ", "घाण पाणी", "पिवळे पाणी", "काळे पाणी", "अळ्या", "दूषित पाणी",
		"మురికి", "బురద", "రంగు మారిన", "కలుషిత", "పురుగులు",
		"அழுக்கு", "கலங்கிய", "மாசு",
		"নোংরা", "ঘোলা", "দূষিত",
		"ગંદુ", "ડહોળું", "દૂષિત",
		"ಕೊಳಕು", "ಕಲುಷಿತ",
	},
	domain.IssueNoWater: {
		"no water", "not coming", "no supply", "supply stopped", "water not", "dry tap", "no pressure", "low pressure",
		"pani nahi", "paani nahi", "pani nahin", "supply band",
		"पानी नहीं", "पानी नही", "सप्लाई बंद", "नल सूखा", "पानी बंद", "प्रेशर कम",
		"पाणी येत नाही", "पाणी नाही", "पाणी बंद",
		"నీళ్ళు రావడం లేదు", "నీరు రావడం లేదు", "నీరు రాలేదు", "నీళ్లు రావట్లేదు",
		"தண்ணீர் வரவில்லை", "தண்ணீர் இல்லை",
		"জল আসছে না", "জল নেই",
		"પાણી નથી", "પાણી આવતું નથી",
		"ನೀರು ಬರುತ್ತಿಲ್ಲ", "ನೀರಿಲ್ಲ",
	},
	domain.IssueLeak: {
		"leak", "leaking", "leakage", "burst", "pipe broken", "broken pipe", "pipeline broken", "overflow",
		"pipe phata", "pipe phut", "pipe toot", "pipe tuta",
		"लीकेज", "लीक", "रिसाव", "पाइप फटा", "पाइप फूटा", "टूटी पाइप", "पाइपलाइन टूटी", "पाइप टूटा",
		"गळती", "पाईप फुटला", "पाइप फुटली",
		"లీక్", "పైపు పగిలింది", "లీకేజీ",
		"கசிவு", "குழாய் உடைந்து",
		"লিক", "পাইপ ফেটে",
		"લીકેજ", "પાઇપ તૂટી",
		"ಸೋರಿಕೆ", "ಪೈಪ್ ಒಡೆದು",
	},
}

// illnessCues indicate residents report waterborne illness symptoms.
var illnessCues = []string{
	"diarrhoea", "diarrhea", "loose motion", "loose motions", "vomit", "vomiting", "sick", "ill", "cholera", "jaundice", "typhoid",
	"stomach pain", "fever", "dehydration", "hospital", "admitted",
	"dast", "ulti", "ultee", "bimar", "beemar", "piliya", "pet dard", "haija",
	"दस्त", "उल्टी", "उलटी", "बीमार", "पीलिया", "हैजा", "टाइफाइड", "पेट दर्द", "बुखार", "अस्पताल", "भर्ती", "डायरिया",
	"जुलाब", "उलट्या", "आजारी", "कावीळ", "ताप", "दवाखान्यात",
	"విరేచనాలు", "వాంతులు", "జ్వరం", "అనారోగ్యం", "ఆసుపత్రి", "కామెర్లు",
	"வயிற்றுப்போக்கு", "வாந்தி", "காய்ச்சல்", "மருத்துவமனை",
	"ডায়রিয়া", "বমি", "অসুস্থ", "জ্বর", "হাসপাতাল",
	"ઝાડા", "ઉલટી", "બીમાર", "તાવ",
	"ಅತಿಸಾರ", "ವಾಂತಿ", "ಜ್ವರ", "ಆಸ್ಪತ್ರೆ",
}

// diarrhoeaCues and vomitingCues separate the two symptoms for illness reports.
var diarrhoeaCues = []string{"diarrhoea", "diarrhea", "loose motion", "dast", "दस्त", "डायरिया", "जुलाब", "విరేచనాలు", "வயிற்றுப்போக்கு", "ডায়রিয়া", "ઝાડા", "ಅತಿಸಾರ", "cholera", "हैजा", "haija"}
var vomitingCues = []string{"vomit", "ulti", "ultee", "उल्टी", "उलटी", "उलट्या", "వాంతులు", "வாந்தி", "বমি", "ઉલટી", "ವಾಂತಿ"}

// urgencyCues raise severity: vulnerable people, many households, deaths.
var urgencyCues = []string{
	"children", "child", "kids", "baby", "elderly", "old", "everyone", "whole street", "whole area", "all houses", "many houses", "death", "died", "dead",
	"bacche", "bachche", "sabke", "sab ghar", "poore mohalle", "pura mohalla", "maut", "mar gaye",
	"बच्चे", "बच्चों", "बुजुर्ग", "सभी घर", "सबके", "पूरे मोहल्ले", "पूरा मोहल्ला", "मौत", "मृत्यु", "मर गए", "गंभीर",
	"मुले", "सगळ्या", "सर्व घरात", "मृत्यू",
	"పిల్లలు", "అందరికీ", "మరణం",
	"குழந்தைகள்", "இறப்பு",
	"শিশু", "মৃত্যু",
}

// durationCues indicate the problem has persisted (days/weeks/months).
var durationCues = []string{
	"days", "weeks", "months", "since", "for a week", "for a month", "din se", "hafte se", "mahine se", "mahino se",
	"दिन से", "दिनों से", "हफ्ते से", "महीने से", "महीनों से", "दिवसांपासून", "महिन्यांपासून",
	"రోజులుగా", "నెలలుగా", "நாட்களாக", "দিন ধরে",
}
