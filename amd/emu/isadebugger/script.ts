class Tag {
  time: number;
  content: string;
}
class Project {
  currentPage: number;
  data: Array<Object>;
  unsaved: boolean;
  tags: Object;
  validated: Array<boolean>;

  constructor() {
    this.currentPage = 0;
    this.tags = {};
  }

  loadTrace(data: string) {
    this.data = JSON.parse(data);
    this.validated = new Array<boolean>();
    for (let i = 0; i < this.data.length; i++) {
      this.validated.push(false);
    }

    preparePage();
    showPage(project.data, 0);
  }

  save() {
    this.unsaved = false;

    var a = document.createElement("a");
    var file = new Blob([JSON.stringify(this)], { type: "text/plain" });
    a.href = URL.createObjectURL(file);
    a.download = "save.json";
    a.click();
  }

  load(content: string) {
    const all = JSON.parse(content);
    this.currentPage = all["currentPage"];
    this.data = all["data"];
    this.tags = all["tags"];
    this.validated = all["validated"];
    this.unsaved = false;
    preparePage();
    showPage(project.data, 0);
  }

  addTag(regName: string, time: number, content: string) {
    if (!this.tags[regName]) {
      this.tags[regName] = new Array<Tag>();
    }

    const registerTags: Array<Tag> = this.tags[regName];

    const tag = new Tag();
    tag.time = time;
    tag.content = content;
    registerTags.push(tag);
    this.tags[regName] = registerTags.sort((t: Tag) => t.time);
  }

  searchTag(regName: string, time: number): string {
    if (!this.tags[regName]) {
      return "";
    }

    const registerTags: Array<Tag> = this.tags[regName];

    for (let i = registerTags.length - 1; i >= 0; i--) {
      if (registerTags[i].time <= time) {
        return registerTags[i].content;
      }
    }

    return "";
  }
}

let project = new Project();

document.getElementById("prev-button").onclick = prevPage;
document.getElementById("next-button").onclick = nextPage;
document.getElementById("validate-button").onclick = validatePage;
document.getElementById("new-button").onclick = newProject;
document.getElementById("save-button").onclick = saveProject;
document.getElementById("load-button").onclick = loadProject;

function nextPage() {
  project.currentPage++;
  if (project.currentPage >= project.data.length) {
    project.currentPage = project.data.length - 1;
  }
  showPage(project.data, project.currentPage);
}

function prevPage() {
  project.currentPage--;
  if (project.currentPage < 0) {
    project.currentPage = 0;
  }

  showPage(project.data, project.currentPage);
}

function validatePage() {
  project.validated[project.currentPage] = !project.validated[
    project.currentPage
  ];
  updateValidateButton();
  updateValidationProgressBar();
}

function updateValidateButton() {
  const btn = document.getElementById("validate-button");
  if (project.validated[project.currentPage]) {
    btn.classList.remove("btn-outline-success");
    btn.classList.add("btn-success");
  } else {
    btn.classList.add("btn-outline-success");
    btn.classList.remove("btn-success");
  }
}

function initializeValidationProgressBar() {
  const bar = document.getElementById("validation-progress-bar");
  for (let i = 0; i < project.data.length; i++) {
    const block = document.createElement("div");
    block.style.width =
      (bar.clientWidth / project.data.length).toString() + "px";
    block.classList.add("added");
    bar.appendChild(block);

    block.onclick = () => {
      project.currentPage = i;
      showPage(project.data, i);
    };
  }
}

function updateValidationProgressBar() {
  const bar = document.getElementById("validation-progress-bar");
  const blocks = bar.getElementsByTagName("*");
  for (let i = 0; i < project.validated.length; i++) {
    if (project.validated[i]) {
      blocks[i].classList.add("validated");
    } else {
      blocks[i].classList.remove("validated");
    }

    if (i == project.currentPage) {
      blocks[i].classList.add("current-inst");
    } else {
      blocks[i].classList.remove("current-inst");
    }
  }
}

interface HTMLInputEvent extends Event {
  target: HTMLInputElement & EventTarget;
}

function newProject() {
  var input = document.createElement("input");
  input.type = "file";
  input.onchange = (e: HTMLInputEvent) => {
    // console.log(e);
    let file = e.target.files[0];

    var reader = new FileReader();
    reader.readAsText(file, "UTF-8");

    // here we tell the reader what to do when it's done reading...
    reader.onload = (readerEvent) => {
      var content = readerEvent.target.result; // this is the content!
      project.loadTrace(content.toString());
    };
  };
  input.click();
}

function saveProject() {
  project.save();
}

function loadProject() {
  var input = document.createElement("input");
  input.type = "file";
  input.onchange = (e: HTMLInputEvent) => {
    // console.log(e);
    let file = e.target.files[0];

    var reader = new FileReader();
    reader.readAsText(file, "UTF-8");

    // here we tell the reader what to do when it's done reading...
    reader.onload = (readerEvent) => {
      var content = readerEvent.target.result; // this is the content!
      project.load(content.toString());
    };
  };
  input.click();
}

function clearPage() {
  let added = document.getElementsByClassName("added");
  for (let i = added.length - 1; i >= 0; i--) {
    let e = added.item(i);
    e.remove();
  }
}

function preparePage() {
  clearPage();
  initializeValidationProgressBar();
  initiateSGPRsTable();
  initiateVGPRsTable();
}

function initiateSGPRsTable() {
  for (let i = 0; i < project.data[0]["SGPRs"].length; i++) {
    const regName = "s" + i.toString();
    const tr = document.createElement("tr");
    tr.classList.add("added");

    const th = document.createElement("th");
    th.scope = "row";
    th.innerHTML = regName;
    tr.appendChild(th);

    const tdTag = document.createElement("td");
    tdTag.classList.add("text-sm-right");
    tr.appendChild(tdTag);

    addTagInput(regName, tdTag);

    const td = document.createElement("td");
    td.id = regName + "-value";
    td.classList.add("text-sm-right");
    tr.appendChild(td);

    document.getElementById("scalar-register-tbody").appendChild(tr);
  }
}

function initiateVGPRsTable() {
  const tbody = document.getElementById("vgpr-tbody");
  initializeVGPRLaneHeaders();
  initiateEXECRow(tbody);
  initiateVCCRow(tbody);
  initiateVGPRRows(tbody);
}

function initializeVGPRLaneHeaders() {
  const headerTr = document.getElementById("vgpr-header-tr");
  for (let i = 0; i < 64; i++) {
    const th = document.createElement("th");
    th.classList.add("text-sm-right");
    th.classList.add("added");
    th.scope = "col";
    th.innerHTML = i.toString();
    headerTr.appendChild(th);
  }
}

function initiateVGPRRows(tbody: HTMLElement) {
  for (let i = 0; i < project.data[0]["VGPRs"].length; i++) {
    const regName = "v" + i.toString();
    const tr = document.createElement("tr");
    tr.classList.add("added");
    const th = document.createElement("th");
    th.innerHTML = regName;
    tr.appendChild(th);
    const tdTag = document.createElement("td");
    tdTag.classList.add("text-sm-right");
    addTagInput(regName, tdTag);
    tr.appendChild(tdTag);

    for (let j = 0; j < 64; j++) {
      const td = document.createElement("td");
      td.classList.add("text-sm-right");
      td.id = regName + "-lane" + j.toString() + "-value";
      tr.append(td);
    }
    tbody.appendChild(tr);
  }
}

function addTagInput(regName: string, tdTag: HTMLTableDataCellElement) {
  const tagInput = document.createElement("input");
  tagInput.id = regName + "-tag-input";
  tagInput.classList.add("reg-tag-input");
  tagInput.type = "text";
  tdTag.appendChild(tagInput);
  tagInput.addEventListener("keyup", (ev: KeyboardEvent) => {
    if (ev.keyCode == 13) {
      const tagContent = (<HTMLInputElement>ev.target).value;
      project.addTag(regName, project.currentPage, tagContent);
    }
  });
}

function initiateVCCRow(tbody: HTMLElement) {
  const tr = document.createElement("tr");
  tr.classList.add("added");
  const th = document.createElement("th");
  th.innerHTML = "VCC";
  tr.appendChild(th);
  const tdTag = document.createElement("td");
  tdTag.classList.add("text-sm-right");
  tr.appendChild(tdTag);
  for (let j = 0; j < 64; j++) {
    const td = document.createElement("td");
    td.classList.add("text-sm-right");
    td.id = "VCC-lane" + j.toString() + "-value";
    tr.append(td);
  }
  tbody.appendChild(tr);
}

function initiateEXECRow(tbody: HTMLElement) {
  const tr = document.createElement("tr");
  tr.classList.add("added");
  const th = document.createElement("th");
  th.innerHTML = "EXEC";
  tr.appendChild(th);
  const tdTag = document.createElement("td");
  tdTag.classList.add("text-sm-right");
  tr.appendChild(tdTag);
  for (let j = 0; j < 64; j++) {
    const td = document.createElement("td");
    td.classList.add("text-sm-right");
    td.id = "EXEC-lane" + j.toString() + "-value";
    tr.append(td);
  }
  tbody.appendChild(tr);
}

function showPage(data: Array<Object>, pageId: number) {
  document.getElementById(
    "current-page-number"
  ).innerHTML = project.currentPage.toString();
  updateValidateButton();
  updateValidationProgressBar();
  showData(data[pageId]);
}

function showData(data: Object) {
  updateValue("pc", "0x" + (data["PCHi"] + data["PCLo"]).toString(16));
  updateValue("inst", data["Inst"]);
  updateValue("scc", data["SCC"]);

  showVCC(data);
  showEXEC(data);
  showSGPRs(data);
  showVGPRs(data);
}

function showVCC(data: Object) {
  const vccLo: number = data["VCCLo"];
  const vccHi: number = data["VCCHi"];
  for (let i = 0; i < 64; i++) {
    let bit = 0;
    if (i >= 32) {
      bit = vccHi & (1 << (i - 32));
    } else {
      bit = vccLo & (1 << i);
    }

    if (bit) {
      updateValue("VCC-lane" + i.toString(), "1");
    } else {
      updateValue("VCC-lane" + i.toString(), "0");
    }
  }
}

function showEXEC(data: Object) {
  const execLo: number = data["EXECLo"];
  const execHi: number = data["EXECHi"];
  for (let i = 0; i < 64; i++) {
    let bit = 0;
    if (i >= 32) {
      bit = execHi & (1 << (i - 32));
    } else {
      bit = execLo & (1 << i);
    }

    if (bit) {
      updateValue("EXEC-lane" + i.toString(), "1");
    } else {
      updateValue("EXEC-lane" + i.toString(), "0");
    }
  }
}

function showSGPRs(data: Object) {
  for (let i = 0; i < data["SGPRs"].length; i++) {
    updateValue("s" + i.toString(), data["SGPRs"][i].toString(16));
    showTags("s" + i.toString());
  }
}

function showVGPRs(data: Object) {
  for (let i = 0; i < data["VGPRs"].length; i++) {
    for (let j = 0; j < data["VGPRs"][i].length; j++) {
      updateValue(
        "v" + i.toString() + "-lane" + j.toString(),
        data["VGPRs"][i][j].toString(16)
      );
    }
  }
}

function updateValue(content: string, value: string) {
  const element = document.getElementById(content + "-value");

  if (element.innerHTML != value) {
    element.classList.add("changed");
  } else {
    element.classList.remove("changed");
  }

  element.innerHTML = value;
}

function showTags(content: string) {
  let tagInput = <HTMLInputElement>(
    document.getElementById(content + "-tag-input")
  );

  let tag = project.searchTag(content, project.currentPage);
  tagInput.value = tag;
}
