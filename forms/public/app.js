const role = location.pathname.slice(1);
const statusLine = document.getElementById("status");
async function init() {
  const response = await fetch("/" + role + ".json");
  if (!response.ok) throw Error("Не удалось загрузить анкету.");
  const model = new Survey.Model(await response.json());
  const note = document.createElement("p");
  note.textContent = "Куратор HR увидит ваши оценки и комментарии вместе с кодом участника и месяцем. Они помогут выбрать вопрос для разговора и проверить, помогла ли договорённость.";
  document.getElementById("form").before(note);
  model.locale = "ru";
  model.showCompletedPage = false;
  const id = crypto.randomUUID();
  let posting = false;
  let saved = false;
  model.onCompleting.add((sender, event) => {
    if (saved) return;
    event.allow = false;
    if (posting) return;
    posting = true;
    statusLine.textContent = "Отправляем ответ…";
    const {intern_id, mentor_id, track, period, ...answers} = sender.data;
    fetch("/submit", {method:"POST", headers:{"Content-Type":"application/json"}, body:JSON.stringify({
      id, role, intern:intern_id, mentor:mentor_id || "", track, period, answers
    })}).then(async result => {
      if (!result.ok) throw Error(await result.text());
      saved = true;
      sender.doComplete();
      document.getElementById("form").hidden = true;
      statusLine.textContent = result.status === 202 ? "Анкета сохранена. Сервис повторит доставку, когда дашборд будет доступен." : "Спасибо. Ответ передан в дашборд HR.";
    }).catch(error => {statusLine.textContent = error.message;}).finally(() => {posting = false;});
  });
  model.render(document.getElementById("form"));
}
if (role === "intern" || role === "mentor") {
  document.getElementById("home").hidden = true;
  init().catch(error => {statusLine.textContent = error.message;});
}
