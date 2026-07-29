-- +goose Up
-- +goose StatementBegin
INSERT INTO greeting_templates (user_id, event_type, tone, gender, language, body, is_active)
SELECT * FROM (
  SELECT NULL, 'birthday', 'warm', 'm', 'he', '{name} היקר, יום הולדת שמח! 🎉 שתהיה לך שנה מלאה בבריאות, אושר והצלחה.', 1
  UNION ALL SELECT NULL, 'birthday', 'warm', 'f', 'he', '{name} היקרה, יום הולדת שמח! 🎉 שתהיה לך שנה מלאה בבריאות, אושר והצלחה.', 1
  UNION ALL SELECT NULL, 'birthday', 'warm', 'm', 'he', 'מזל טוב {name}! 🎂 מאחל לך שנה מתוקה, רגועה ומלאה ברגעים טובים.', 1
  UNION ALL SELECT NULL, 'birthday', 'warm', 'f', 'he', 'מזל טוב {name}! 🎂 מאחלת לך שנה מתוקה, רגועה ומלאה ברגעים טובים.', 1
  UNION ALL SELECT NULL, 'birthday', 'warm', 'm', 'he', '{nickname} שלי, יום הולדת שמח מהלב! 💛 שתמיד תישאר כפי שאתה — מיוחד ואהוב.', 1
  UNION ALL SELECT NULL, 'birthday', 'warm', 'f', 'he', '{nickname} שלי, יום הולדת שמח מהלב! 💛 שתמיד תישארי כפי שאת — מיוחדת ואהובה.', 1
  UNION ALL SELECT NULL, 'birthday', 'warm', NULL, 'he', 'יום הולדת שמח {name}! מקווה שהשנה הקרובה תביא לך רק חיוכים ורגעים חמים.', 1
  UNION ALL SELECT NULL, 'birthday', 'warm', NULL, 'he', '{name}, כל שנה איתך היא מתנה. יום הולדת שמח, ושתהיה שנה טובה ומתוקה במיוחד.', 1
  UNION ALL SELECT NULL, 'birthday', 'warm', 'm', 'he', 'מאחל לך {name} יום הולדת מלא אהבה, ולשנה הבאה — בריאות, שלווה והמון רגעים טובים.', 1
  UNION ALL SELECT NULL, 'birthday', 'warm', 'f', 'he', 'מאחלת לך {name} יום הולדת מלא אהבה, ולשנה הבאה — בריאות, שלווה והמון רגעים טובים.', 1
  UNION ALL SELECT NULL, 'birthday', 'funny', NULL, 'he', '{name}, עוד שנה מתווספת לגיל אבל לא לחוכמה 😂 יום הולדת שמח!', 1
  UNION ALL SELECT NULL, 'birthday', 'funny', 'm', 'he', 'מזל טוב {name}! רשמית אתה עכשיו מבוגר מדי בשביל השטויות שאתה עדיין עושה 🎈', 1
  UNION ALL SELECT NULL, 'birthday', 'funny', 'f', 'he', 'מזל טוב {name}! רשמית את עכשיו מבוגרת מדי בשביל השטויות שאת עדיין עושה 🎈', 1
  UNION ALL SELECT NULL, 'birthday', 'funny', 'm', 'he', 'יום הולדת שמח {nickname}! תזכור — הגיל זה רק מספר, במיוחד כשמתעגלים כלפי מטה 😉', 1
  UNION ALL SELECT NULL, 'birthday', 'funny', 'f', 'he', 'יום הולדת שמח {nickname}! תזכרי — הגיל זה רק מספר, במיוחד כשמתעגלים כלפי מטה 😉', 1
  UNION ALL SELECT NULL, 'birthday', 'funny', NULL, 'he', '{name}, עוד שנה, עוד עוגה, עוד תירוץ לא ללכת לחדר כושר. מזל טוב! 🍰', 1
  UNION ALL SELECT NULL, 'birthday', 'funny', NULL, 'he', 'כל הכבוד {name} על עוד שנה שרדת אותנו! יום הולדת שמח 🥳', 1
  UNION ALL SELECT NULL, 'birthday', 'funny', NULL, 'he', 'מזל טוב {name}! היום מותר הכל — גם עוגה שנייה וגם לא לענות להודעות.', 1
  UNION ALL SELECT NULL, 'birthday', 'formal', 'm', 'he', '{name} היקר, ברצוני לאחל לך יום הולדת שמח ושנה של הצלחה ובריאות טובה.', 1
  UNION ALL SELECT NULL, 'birthday', 'formal', 'f', 'he', '{name} היקרה, ברצוני לאחל לך יום הולדת שמח ושנה של הצלחה ובריאות טובה.', 1
  UNION ALL SELECT NULL, 'birthday', 'formal', NULL, 'he', 'מיטב האיחולים ליום הולדתך, {name}. שתהיה שנה פורייה ומוצלחת בכל התחומים.', 1
  UNION ALL SELECT NULL, 'birthday', 'formal', 'm', 'he', 'בהזדמנות זו ברצוני לברך אותך {name} על יום הולדתך, ולאחל לך שנה טובה ומוצלחת.', 1
  UNION ALL SELECT NULL, 'birthday', 'formal', 'f', 'he', 'בהזדמנות זו ברצוני לברך אותך {name} על יום הולדתך, ולאחל לך שנה טובה ומוצלחת.', 1
  UNION ALL SELECT NULL, 'birthday', 'formal', 'm', 'he', '{name}, יום הולדת שמח. מאחל לך שביעות רצון, בריאות והגשמת מטרות בשנה הקרובה.', 1
  UNION ALL SELECT NULL, 'birthday', 'formal', 'f', 'he', '{name}, יום הולדת שמח. מאחלת לך שביעות רצון, בריאות והגשמת מטרות בשנה הקרובה.', 1
  UNION ALL SELECT NULL, 'birthday', 'formal', 'm', 'he', 'לכבוד יום הולדתך, {name}, הרשה לי לאחל לך אך ורק טוב בשנה הבאה.', 1
  UNION ALL SELECT NULL, 'birthday', 'formal', 'f', 'he', 'לכבוד יום הולדתך, {name}, הרשי לי לאחל לך אך ורק טוב בשנה הבאה.', 1
  UNION ALL SELECT NULL, 'birthday', 'formal', NULL, 'he', 'ברכות חמות ליום הולדתך {name}, ומיטב האיחולים להמשך הדרך.', 1
  UNION ALL SELECT NULL, 'birthday', 'short', NULL, 'he', 'יום הולדת שמח {name}! 🎂', 1
  UNION ALL SELECT NULL, 'birthday', 'short', NULL, 'he', 'מזל טוב {name}! 🎉', 1
  UNION ALL SELECT NULL, 'birthday', 'short', NULL, 'he', '{name}, יום הולדת שמח ומתוק 🎈', 1
  UNION ALL SELECT NULL, 'birthday', 'short', 'm', 'he', 'מאחל לך {name} שנה נהדרת! 🎁', 1
  UNION ALL SELECT NULL, 'birthday', 'short', 'f', 'he', 'מאחלת לך {name} שנה נהדרת! 🎁', 1
  UNION ALL SELECT NULL, 'birthday', 'short', NULL, 'he', 'יום הולדת שמח {nickname}! 🥳', 1
  UNION ALL SELECT NULL, 'birthday', 'short', NULL, 'he', '{name} — יום הולדת שמח! ❤️', 1
  UNION ALL SELECT NULL, 'anniversary', 'warm', 'm', 'he', '{name} היקר, מזל טוב ליום הנישואין! מאחל לכם המשך אהבה ואושר.', 1
  UNION ALL SELECT NULL, 'anniversary', 'warm', 'f', 'he', '{name} היקרה, מזל טוב ליום הנישואין! מאחלת לכם המשך אהבה ואושר.', 1
  UNION ALL SELECT NULL, 'anniversary', 'warm', NULL, 'he', 'יום נישואין שמח {name}! שתמשיכו לצמוח ולהתחזק יחד, שנה אחרי שנה.', 1
  UNION ALL SELECT NULL, 'anniversary', 'warm', NULL, 'he', '{name}, כל השנים הזוגיות שלכם הן השראה. מזל טוב ליום המיוחד הזה 💍', 1
  UNION ALL SELECT NULL, 'anniversary', 'warm', 'm', 'he', 'מאחל לך {name} ולבת הזוג עוד שנים רבות של אהבה ושותפות אמיתית.', 1
  UNION ALL SELECT NULL, 'anniversary', 'warm', 'f', 'he', 'מאחלת לך {name} ולבן הזוג עוד שנים רבות של אהבה ושותפות אמיתית.', 1
  UNION ALL SELECT NULL, 'anniversary', 'warm', NULL, 'he', 'יום נישואין שמח! {name}, שתמיד תזכרו למה בחרתם זה בזה מלכתחילה.', 1
  UNION ALL SELECT NULL, 'anniversary', 'warm', NULL, 'he', '{name}, מזל טוב ליום הנישואין. האהבה שלכם היא דבר יפה לראות.', 1
  UNION ALL SELECT NULL, 'anniversary', 'funny', NULL, 'he', '{name}, עוד שנה ששרדתם אחד את השני — כל הכבוד! מזל טוב ליום הנישואין 😄', 1
  UNION ALL SELECT NULL, 'anniversary', 'funny', NULL, 'he', 'מזל טוב {name}! עוד שנה של ויכוחים על הטמפרטורה במזגן, וזה עדיין עובד 😂', 1
  UNION ALL SELECT NULL, 'anniversary', 'funny', NULL, 'he', 'יום נישואין שמח {name}! מי היה מאמין שהחוזה עוד בתוקף.', 1
  UNION ALL SELECT NULL, 'anniversary', 'funny', NULL, 'he', '{name}, מזל טוב! עוד שנה יחד — הביטוח על הזוגיות שלכם משתלם.', 1
  UNION ALL SELECT NULL, 'anniversary', 'funny', NULL, 'he', 'כל הכבוד {name} על עוד שנה של סבלנות הדדית. יום נישואין שמח!', 1
  UNION ALL SELECT NULL, 'anniversary', 'funny', NULL, 'he', 'מזל טוב {name}! רשמית עברתם עוד שנה בלי לרצוח אחד את השני 🎉', 1
  UNION ALL SELECT NULL, 'anniversary', 'formal', 'm', 'he', '{name} היקר, ברכותיי החמות ליום הנישואין. מאחל לכם המשך דרך משותפת ומוצלחת.', 1
  UNION ALL SELECT NULL, 'anniversary', 'formal', 'f', 'he', '{name} היקרה, ברכותיי החמות ליום הנישואין. מאחלת לכם המשך דרך משותפת ומוצלחת.', 1
  UNION ALL SELECT NULL, 'anniversary', 'formal', 'm', 'he', 'בהזדמנות יום נישואיכם, {name}, הרשה לי לאחל לכם אושר ובריאות.', 1
  UNION ALL SELECT NULL, 'anniversary', 'formal', 'f', 'he', 'בהזדמנות יום נישואיכם, {name}, הרשי לי לאחל לכם אושר ובריאות.', 1
  UNION ALL SELECT NULL, 'anniversary', 'formal', NULL, 'he', 'מיטב האיחולים ליום הנישואין, {name}. שתזכו לשנים רבות נוספות יחד.', 1
  UNION ALL SELECT NULL, 'anniversary', 'formal', NULL, 'he', '{name}, ברכה לבבית ליום המיוחד הזה. שתמשיכו להצליח יחד בכל תחום.', 1
  UNION ALL SELECT NULL, 'anniversary', 'formal', 'm', 'he', 'לרגל יום הנישואין, {name}, מאחל לכם הרמוניה והמשך שגשוג משותף.', 1
  UNION ALL SELECT NULL, 'anniversary', 'formal', 'f', 'he', 'לרגל יום הנישואין, {name}, מאחלת לכם הרמוניה והמשך שגשוג משותף.', 1
  UNION ALL SELECT NULL, 'anniversary', 'formal', NULL, 'he', 'ברכות ליום הנישואין {name}. תודה על ההשראה שאתם נותנים לסובבים אתכם.', 1
  UNION ALL SELECT NULL, 'anniversary', 'short', NULL, 'he', 'יום נישואין שמח {name}! 💍', 1
  UNION ALL SELECT NULL, 'anniversary', 'short', NULL, 'he', 'מזל טוב {name}! 💐', 1
  UNION ALL SELECT NULL, 'anniversary', 'short', NULL, 'he', '{name}, מזל טוב ליום המיוחד 💕', 1
  UNION ALL SELECT NULL, 'anniversary', 'short', NULL, 'he', 'יום נישואין שמח! ❤️', 1
  UNION ALL SELECT NULL, 'anniversary', 'short', NULL, 'he', '{name} — מזל טוב לזוגיות! 🥂', 1
  UNION ALL SELECT NULL, 'anniversary', 'short', 'm', 'he', 'מאחל לכם עוד שנים יחד 💫', 1
  UNION ALL SELECT NULL, 'anniversary', 'short', 'f', 'he', 'מאחלת לכם עוד שנים יחד 💫', 1
  UNION ALL SELECT NULL, 'wedding', 'warm', 'm', 'he', '{name} היקר, מזל טוב לחתונה! שתבנו יחד בית מלא אהבה ושמחה.', 1
  UNION ALL SELECT NULL, 'wedding', 'warm', 'f', 'he', '{name} היקרה, מזל טוב לחתונה! שתבנו יחד בית מלא אהבה ושמחה.', 1
  UNION ALL SELECT NULL, 'wedding', 'warm', 'm', 'he', 'איזה יום מרגש, {name}! מאחל לכם חיים משותפים מלאי אושר ובריאות.', 1
  UNION ALL SELECT NULL, 'wedding', 'warm', 'f', 'he', 'איזה יום מרגש, {name}! מאחלת לכם חיים משותפים מלאי אושר ובריאות.', 1
  UNION ALL SELECT NULL, 'wedding', 'warm', NULL, 'he', 'מזל טוב {name}! שתמיד תדעו לתמוך אחד בשני ולצמוח יחד.', 1
  UNION ALL SELECT NULL, 'wedding', 'warm', 'm', 'he', '{name}, יום החתונה שלכם הוא רק ההתחלה. מאחל לכם דרך משותפת נפלאה.', 1
  UNION ALL SELECT NULL, 'wedding', 'warm', 'f', 'he', '{name}, יום החתונה שלכם הוא רק ההתחלה. מאחלת לכם דרך משותפת נפלאה.', 1
  UNION ALL SELECT NULL, 'wedding', 'warm', NULL, 'he', 'כל הברכות לחתונה, {name}! שתחיו באהבה, בכבוד ובשמחה כל ימי חייכם.', 1
  UNION ALL SELECT NULL, 'wedding', 'warm', NULL, 'he', 'מזל טוב {name}! שהבית החדש שלכם יהיה מלא אור, אהבה וצחוק.', 1
  UNION ALL SELECT NULL, 'wedding', 'funny', NULL, 'he', '{name}, מזל טוב לחתונה! עכשיו רשמית אין דרך חזרה 😂', 1
  UNION ALL SELECT NULL, 'wedding', 'funny', 'm', 'he', 'ברוך הבא למועדון הנשואים, {name}! החליפה יקרה, האושר בחינם.', 1
  UNION ALL SELECT NULL, 'wedding', 'funny', 'f', 'he', 'ברוכה הבאה למועדון הנשואים, {name}! השמלה יקרה, האושר בחינם.', 1
  UNION ALL SELECT NULL, 'wedding', 'funny', NULL, 'he', 'מזל טוב {name}! שיהיה לכם בהצלחה עם הוויכוחים הראשונים על הריהוט.', 1
  UNION ALL SELECT NULL, 'wedding', 'funny', NULL, 'he', '{name}, סוף סוף חתונה! עכשיו אפשר להפסיק לשאול מתי.', 1
  UNION ALL SELECT NULL, 'wedding', 'funny', 'm', 'he', 'מזל טוב {name}! מאחל לכם שהצלחת התזמורת תעלה על מחיר האולם.', 1
  UNION ALL SELECT NULL, 'wedding', 'funny', 'f', 'he', 'מזל טוב {name}! מאחלת לכם שהצלחת התזמורת תעלה על מחיר האולם.', 1
  UNION ALL SELECT NULL, 'wedding', 'funny', 'm', 'he', 'כל הכבוד {name}! מצאת מישהי שסובלת אותך רשמית ולכל החיים 😄', 1
  UNION ALL SELECT NULL, 'wedding', 'funny', 'f', 'he', 'כל הכבוד {name}! מצאת מישהו שסובל אותך רשמית ולכל החיים 😄', 1
  UNION ALL SELECT NULL, 'wedding', 'formal', 'm', 'he', '{name} היקר, ברכותיי החמות לרגל נישואיך. מאחל לך אושר ובריאות.', 1
  UNION ALL SELECT NULL, 'wedding', 'formal', 'f', 'he', '{name} היקרה, ברכותיי החמות לרגל נישואייך. מאחלת לך אושר ובריאות.', 1
  UNION ALL SELECT NULL, 'wedding', 'formal', 'm', 'he', 'לרגל יום חתונתך, {name}, הרשה לי לאחל לך חיי זוגיות מוצלחים ומאושרים.', 1
  UNION ALL SELECT NULL, 'wedding', 'formal', 'f', 'he', 'לרגל יום חתונתך, {name}, הרשי לי לאחל לך חיי זוגיות מוצלחים ומאושרים.', 1
  UNION ALL SELECT NULL, 'wedding', 'formal', 'm', 'he', 'מיטב האיחולים לחתונתך, {name}. שתזכה לבית נאמן ומלא ברכה.', 1
  UNION ALL SELECT NULL, 'wedding', 'formal', 'f', 'he', 'מיטב האיחולים לחתונתך, {name}. שתזכי לבית נאמן ומלא ברכה.', 1
  UNION ALL SELECT NULL, 'wedding', 'formal', NULL, 'he', '{name}, ברכה לבבית לרגל הקמת הבית החדש. שתצליחו בכל דרככם המשותפת.', 1
  UNION ALL SELECT NULL, 'wedding', 'formal', 'm', 'he', 'ברכות לחתונתך {name}. שתהיה זו תחילתם של חיים משותפים ומוצלחים.', 1
  UNION ALL SELECT NULL, 'wedding', 'formal', 'f', 'he', 'ברכות לחתונתך {name}. שתהיה זו תחילתן של שנים משותפות ומוצלחות.', 1
  UNION ALL SELECT NULL, 'wedding', 'formal', 'm', 'he', 'לרגל השמחה, {name}, מאחל לך ולבת הזוג שנים רבות של אהבה ושגשוג.', 1
  UNION ALL SELECT NULL, 'wedding', 'formal', 'f', 'he', 'לרגל השמחה, {name}, מאחלת לך ולבן הזוג שנים רבות של אהבה ושגשוג.', 1
  UNION ALL SELECT NULL, 'wedding', 'short', NULL, 'he', 'מזל טוב לחתונה {name}! 💒', 1
  UNION ALL SELECT NULL, 'wedding', 'short', NULL, 'he', '{name}, מזל טוב! 🎊', 1
  UNION ALL SELECT NULL, 'wedding', 'short', NULL, 'he', 'איזו שמחה, {name}! 💐', 1
  UNION ALL SELECT NULL, 'wedding', 'short', NULL, 'he', 'מזל טוב לזוג המאושר! 🥂', 1
  UNION ALL SELECT NULL, 'wedding', 'short', NULL, 'he', '{name} — מזל טוב לחתונה! 💍', 1
  UNION ALL SELECT NULL, 'wedding', 'short', NULL, 'he', 'כל הברכות לחתונה 🎉', 1
  UNION ALL SELECT NULL, 'custom', 'warm', 'm', 'he', '{name} היקר, מאחל לך המון שמחה ואושר ביום המיוחד הזה.', 1
  UNION ALL SELECT NULL, 'custom', 'warm', 'f', 'he', '{name} היקרה, מאחלת לך המון שמחה ואושר ביום המיוחד הזה.', 1
  UNION ALL SELECT NULL, 'custom', 'warm', NULL, 'he', 'יום נפלא לך {name}! שיהיה מלא ברגעים טובים וחמים.', 1
  UNION ALL SELECT NULL, 'custom', 'warm', NULL, 'he', '{name}, מקווה שהיום הזה מביא לך רק דברים טובים.', 1
  UNION ALL SELECT NULL, 'custom', 'warm', 'm', 'he', 'מאחל לך {name} יום מיוחד כמו שאתה.', 1
  UNION ALL SELECT NULL, 'custom', 'warm', 'f', 'he', 'מאחלת לך {name} יום מיוחד כמו שאת.', 1
  UNION ALL SELECT NULL, 'custom', 'warm', NULL, 'he', '{name}, שיהיה לך יום נפלא מלא אהבה מהסובבים אותך.', 1
  UNION ALL SELECT NULL, 'custom', 'warm', 'm', 'he', 'כל טוב לך {name} ביום החשוב הזה. אתה ראוי לכל הטוב שבעולם.', 1
  UNION ALL SELECT NULL, 'custom', 'warm', 'f', 'he', 'כל טוב לך {name} ביום החשוב הזה. את ראויה לכל הטוב שבעולם.', 1
  UNION ALL SELECT NULL, 'custom', 'funny', NULL, 'he', '{name}, יום מיוחד מגיע גם עוגה מיוחדת. תתפנק!', 1
  UNION ALL SELECT NULL, 'custom', 'funny', 'm', 'he', 'מזל טוב {name}! היום אתה רשמית פטור מלעשות דברים רציניים.', 1
  UNION ALL SELECT NULL, 'custom', 'funny', 'f', 'he', 'מזל טוב {name}! היום את רשמית פטורה מלעשות דברים רציניים.', 1
  UNION ALL SELECT NULL, 'custom', 'funny', 'm', 'he', '{name}, מקווה שהיום שלך יהיה טוב יותר מהתירוצים שאתה נותן בדרך כלל.', 1
  UNION ALL SELECT NULL, 'custom', 'funny', 'f', 'he', '{name}, מקווה שהיום שלך יהיה טוב יותר מהתירוצים שאת נותנת בדרך כלל.', 1
  UNION ALL SELECT NULL, 'custom', 'funny', 'm', 'he', 'יום מצוין לך {name}! תזכור לחגוג בלי סייגים.', 1
  UNION ALL SELECT NULL, 'custom', 'funny', 'f', 'he', 'יום מצוין לך {name}! תזכרי לחגוג בלי סייגים.', 1
  UNION ALL SELECT NULL, 'custom', 'funny', 'm', 'he', '{name}, היום אתה מקבל רישיון רשמי לבטלנות. תיהנה!', 1
  UNION ALL SELECT NULL, 'custom', 'funny', 'f', 'he', '{name}, היום את מקבלת רישיון רשמי לבטלנות. תיהני!', 1
  UNION ALL SELECT NULL, 'custom', 'funny', NULL, 'he', 'מזל טוב {name}! היום מותר גם קינוח לפני ארוחה.', 1
  UNION ALL SELECT NULL, 'custom', 'formal', 'm', 'he', '{name} היקר, ברצוני לאחל לך יום מוצלח ומהנה.', 1
  UNION ALL SELECT NULL, 'custom', 'formal', 'f', 'he', '{name} היקרה, ברצוני לאחל לך יום מוצלח ומהנה.', 1
  UNION ALL SELECT NULL, 'custom', 'formal', NULL, 'he', 'מיטב האיחולים לך, {name}, ביום המיוחד הזה.', 1
  UNION ALL SELECT NULL, 'custom', 'formal', 'm', 'he', '{name}, מאחל לך יום נעים והגשמת כל מטרותיך.', 1
  UNION ALL SELECT NULL, 'custom', 'formal', 'f', 'he', '{name}, מאחלת לך יום נעים והגשמת כל מטרותייך.', 1
  UNION ALL SELECT NULL, 'custom', 'formal', 'm', 'he', 'בהזדמנות זו ברצוני לברך אותך, {name}, ולאחל לך את הטוב ביותר.', 1
  UNION ALL SELECT NULL, 'custom', 'formal', 'f', 'he', 'בהזדמנות זו ברצוני לברך אותך, {name}, ולאחל לך את הטוב ביותר.', 1
  UNION ALL SELECT NULL, 'custom', 'formal', 'm', 'he', 'ברכותיי החמות לך, {name}, ליום זה.', 1
  UNION ALL SELECT NULL, 'custom', 'formal', 'f', 'he', 'ברכותיי החמות לך, {name}, ליום זה.', 1
  UNION ALL SELECT NULL, 'custom', 'formal', 'm', 'he', '{name}, מאחל לך יום שקט, נעים ומוצלח בכל דרך.', 1
  UNION ALL SELECT NULL, 'custom', 'formal', 'f', 'he', '{name}, מאחלת לך יום שקט, נעים ומוצלח בכל דרך.', 1
  UNION ALL SELECT NULL, 'custom', 'short', NULL, 'he', 'מזל טוב {name}! 🎉', 1
  UNION ALL SELECT NULL, 'custom', 'short', NULL, 'he', 'יום נפלא {name}! ✨', 1
  UNION ALL SELECT NULL, 'custom', 'short', NULL, 'he', '{name}, כל טוב! 🎈', 1
  UNION ALL SELECT NULL, 'custom', 'short', 'm', 'he', 'מאחל לך יום מצוין! 🌟', 1
  UNION ALL SELECT NULL, 'custom', 'short', 'f', 'he', 'מאחלת לך יום מצוין! 🌟', 1
  UNION ALL SELECT NULL, 'custom', 'short', NULL, 'he', '{name} — מזל טוב! 🎊', 1
  UNION ALL SELECT NULL, 'custom', 'short', NULL, 'he', 'שיהיה מדהים, {name}! 💫', 1
  UNION ALL SELECT NULL, 'birthday', 'warm', NULL, 'en', 'Dear {name}, happy birthday! Wishing you a year full of health, happiness and success.', 1
  UNION ALL SELECT NULL, 'birthday', 'warm', NULL, 'en', 'Happy birthday {name}! May this year bring you warmth, joy and beautiful moments.', 1
  UNION ALL SELECT NULL, 'birthday', 'warm', NULL, 'en', '{nickname}, sending you all my love on your birthday. Stay exactly who you are.', 1
  UNION ALL SELECT NULL, 'birthday', 'funny', NULL, 'en', '{name}, another year older, still not wiser. Happy birthday!', 1
  UNION ALL SELECT NULL, 'birthday', 'funny', NULL, 'en', 'Happy birthday {name}! Age is just a number, especially when you round down.', 1
  UNION ALL SELECT NULL, 'birthday', 'funny', NULL, 'en', 'Congrats {name} on surviving another year of us. Happy birthday!', 1
  UNION ALL SELECT NULL, 'birthday', 'formal', NULL, 'en', 'Dear {name}, please accept my warmest wishes on your birthday, for health and success.', 1
  UNION ALL SELECT NULL, 'birthday', 'formal', NULL, 'en', 'Best wishes on your birthday, {name}. May the year ahead be prosperous and fulfilling.', 1
  UNION ALL SELECT NULL, 'birthday', 'formal', NULL, 'en', 'On the occasion of your birthday, {name}, I wish you continued success and good health.', 1
  UNION ALL SELECT NULL, 'birthday', 'short', NULL, 'en', 'Happy birthday {name}! 🎂', 1
  UNION ALL SELECT NULL, 'birthday', 'short', NULL, 'en', 'Happy birthday {name}! 🎉', 1
  UNION ALL SELECT NULL, 'birthday', 'short', NULL, 'en', '{name}, wishing you a wonderful year! 🎈', 1
  UNION ALL SELECT NULL, 'anniversary', 'warm', NULL, 'en', 'Dear {name}, happy anniversary! Wishing you continued love and happiness together.', 1
  UNION ALL SELECT NULL, 'anniversary', 'warm', NULL, 'en', 'Happy anniversary {name}! May you keep growing stronger together, year after year.', 1
  UNION ALL SELECT NULL, 'anniversary', 'warm', NULL, 'en', '{name}, your relationship is truly inspiring. Happy anniversary!', 1
  UNION ALL SELECT NULL, 'anniversary', 'funny', NULL, 'en', '{name}, another year of surviving each other. Congrats on your anniversary!', 1
  UNION ALL SELECT NULL, 'anniversary', 'funny', NULL, 'en', 'Happy anniversary {name}! Still arguing about the thermostat, still going strong.', 1
  UNION ALL SELECT NULL, 'anniversary', 'funny', NULL, 'en', 'Congrats {name}, the contract is still valid! Happy anniversary.', 1
  UNION ALL SELECT NULL, 'anniversary', 'formal', NULL, 'en', 'Dear {name}, my warmest congratulations on your anniversary. Wishing you continued happiness.', 1
  UNION ALL SELECT NULL, 'anniversary', 'formal', NULL, 'en', 'On the occasion of your anniversary, {name}, I wish you good health and happiness.', 1
  UNION ALL SELECT NULL, 'anniversary', 'formal', NULL, 'en', 'Best wishes on your anniversary, {name}. May you share many more years together.', 1
  UNION ALL SELECT NULL, 'anniversary', 'short', NULL, 'en', 'Happy anniversary {name}! 💍', 1
  UNION ALL SELECT NULL, 'anniversary', 'short', NULL, 'en', 'Congrats {name}! 💐', 1
  UNION ALL SELECT NULL, 'anniversary', 'short', NULL, 'en', '{name}, happy anniversary! ❤️', 1
  UNION ALL SELECT NULL, 'wedding', 'warm', NULL, 'en', 'Dear {name}, congratulations on your wedding! Wishing you a home filled with love and joy.', 1
  UNION ALL SELECT NULL, 'wedding', 'warm', NULL, 'en', 'What a beautiful day, {name}! Wishing you a lifetime of happiness together.', 1
  UNION ALL SELECT NULL, 'wedding', 'warm', NULL, 'en', 'Congratulations {name}! May you always support and grow with one another.', 1
  UNION ALL SELECT NULL, 'wedding', 'funny', NULL, 'en', '{name}, congrats on the wedding! No turning back now 😂', 1
  UNION ALL SELECT NULL, 'wedding', 'funny', NULL, 'en', 'Welcome to the married club, {name}! The outfit was expensive, the happiness is free.', 1
  UNION ALL SELECT NULL, 'wedding', 'funny', NULL, 'en', 'Congrats {name}! Good luck with the first furniture arguments.', 1
  UNION ALL SELECT NULL, 'wedding', 'formal', NULL, 'en', 'Dear {name}, my warmest congratulations on your marriage. Wishing you happiness and health.', 1
  UNION ALL SELECT NULL, 'wedding', 'formal', NULL, 'en', 'On the occasion of your wedding, {name}, I wish you a joyful and successful married life.', 1
  UNION ALL SELECT NULL, 'wedding', 'formal', NULL, 'en', 'Best wishes on your wedding, {name}. May your home be filled with blessing.', 1
  UNION ALL SELECT NULL, 'wedding', 'short', NULL, 'en', 'Congrats on the wedding {name}! 💒', 1
  UNION ALL SELECT NULL, 'wedding', 'short', NULL, 'en', 'Congratulations {name}! 🎊', 1
  UNION ALL SELECT NULL, 'wedding', 'short', NULL, 'en', 'So happy for you, {name}! 💐', 1
  UNION ALL SELECT NULL, 'custom', 'warm', NULL, 'en', 'Dear {name}, wishing you so much joy and happiness on this special day.', 1
  UNION ALL SELECT NULL, 'custom', 'warm', NULL, 'en', 'Have a wonderful day, {name}! Full of warmth and good moments.', 1
  UNION ALL SELECT NULL, 'custom', 'warm', NULL, 'en', '{name}, hoping this day brings you nothing but good things.', 1
  UNION ALL SELECT NULL, 'custom', 'funny', NULL, 'en', '{name}, special day, special cake. Treat yourself!', 1
  UNION ALL SELECT NULL, 'custom', 'funny', NULL, 'en', 'Congrats {name}! You''re officially excused from being serious today.', 1
  UNION ALL SELECT NULL, 'custom', 'funny', NULL, 'en', 'Have a great day {name}! Dessert before dinner is allowed today.', 1
  UNION ALL SELECT NULL, 'custom', 'formal', NULL, 'en', 'Dear {name}, I wish you a wonderful and successful day.', 1
  UNION ALL SELECT NULL, 'custom', 'formal', NULL, 'en', 'Best wishes to you, {name}, on this special day.', 1
  UNION ALL SELECT NULL, 'custom', 'formal', NULL, 'en', '{name}, wishing you a pleasant day and the fulfillment of all your goals.', 1
  UNION ALL SELECT NULL, 'custom', 'short', NULL, 'en', 'Congrats {name}! 🎉', 1
  UNION ALL SELECT NULL, 'custom', 'short', NULL, 'en', 'Wonderful day {name}! ✨', 1
  UNION ALL SELECT NULL, 'custom', 'short', NULL, 'en', 'All the best, {name}! 🎈', 1
)
WHERE NOT EXISTS (SELECT 1 FROM greeting_templates WHERE user_id IS NULL);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM greeting_templates WHERE user_id IS NULL;
-- +goose StatementEnd
