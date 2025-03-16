var Tag = /** @class */ (function () {
    function Tag() {
    }
    return Tag;
}());
var Project = /** @class */ (function () {
    function Project() {
        this.currentPage = 0;
        this.tags = {};
    }
    Project.prototype.loadTrace = function (data) {
        this.data = JSON.parse(data);
        this.validated = new Array();
        for (var i = 0; i < this.data.length; i++) {
            this.validated.push(false);
        }
        preparePage();
        showPage(project.data, 0);
    };
    Project.prototype.save = function () {
        this.unsaved = false;
        var a = document.createElement("a");
        var file = new Blob([JSON.stringify(this)], { type: "text/plain" });
        a.href = URL.createObjectURL(file);
        a.download = "save.json";
        a.click();
    };
    Project.prototype.load = function (content) {
        var all = JSON.parse(content);
        this.currentPage = all["currentPage"];
        this.data = all["data"];
        this.tags = all["tags"];
        this.validated = all["validated"];
        this.unsaved = false;
        preparePage();
        showPage(project.data, 0);
    };
    Project.prototype.addTag = function (regName, time, content) {
        if (!this.tags[regName]) {
            this.tags[regName] = new Array();
        }
        var registerTags = this.tags[regName];
        var tag = new Tag();
        tag.time = time;
        tag.content = content;
        registerTags.push(tag);
        this.tags[regName] = registerTags.sort(function (t) { return t.time; });
    };
    Project.prototype.searchTag = function (regName, time) {
        if (!this.tags[regName]) {
            return "";
        }
        var registerTags = this.tags[regName];
        for (var i = registerTags.length - 1; i >= 0; i--) {
            if (registerTags[i].time <= time) {
                return registerTags[i].content;
            }
        }
        return "";
    };
    return Project;
}());
var project = new Project();
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
    project.validated[project.currentPage] = !project.validated[project.currentPage];
    updateValidateButton();
    updateValidationProgressBar();
}
function updateValidateButton() {
    var btn = document.getElementById("validate-button");
    if (project.validated[project.currentPage]) {
        btn.classList.remove("btn-outline-success");
        btn.classList.add("btn-success");
    }
    else {
        btn.classList.add("btn-outline-success");
        btn.classList.remove("btn-success");
    }
}
function initializeValidationProgressBar() {
    var bar = document.getElementById("validation-progress-bar");
    var _loop_1 = function (i) {
        var block = document.createElement("div");
        block.style.width =
            (bar.clientWidth / project.data.length).toString() + "px";
        block.classList.add("added");
        bar.appendChild(block);
        block.onclick = function () {
            project.currentPage = i;
            showPage(project.data, i);
        };
    };
    for (var i = 0; i < project.data.length; i++) {
        _loop_1(i);
    }
}
function updateValidationProgressBar() {
    var bar = document.getElementById("validation-progress-bar");
    var blocks = bar.getElementsByTagName("*");
    for (var i = 0; i < project.validated.length; i++) {
        if (project.validated[i]) {
            blocks[i].classList.add("validated");
        }
        else {
            blocks[i].classList.remove("validated");
        }
        if (i == project.currentPage) {
            blocks[i].classList.add("current-inst");
        }
        else {
            blocks[i].classList.remove("current-inst");
        }
    }
}
function newProject() {
    var input = document.createElement("input");
    input.type = "file";
    input.onchange = function (e) {
        // console.log(e);
        var file = e.target.files[0];
        var reader = new FileReader();
        reader.readAsText(file, "UTF-8");
        // here we tell the reader what to do when it's done reading...
        reader.onload = function (readerEvent) {
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
    input.onchange = function (e) {
        // console.log(e);
        var file = e.target.files[0];
        var reader = new FileReader();
        reader.readAsText(file, "UTF-8");
        // here we tell the reader what to do when it's done reading...
        reader.onload = function (readerEvent) {
            var content = readerEvent.target.result; // this is the content!
            project.load(content.toString());
        };
    };
    input.click();
}
function clearPage() {
    var added = document.getElementsByClassName("added");
    for (var i = added.length - 1; i >= 0; i--) {
        var e = added.item(i);
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
    for (var i = 0; i < project.data[0]["SGPRs"].length; i++) {
        var regName = "s" + i.toString();
        var tr = document.createElement("tr");
        tr.classList.add("added");
        var th = document.createElement("th");
        th.scope = "row";
        th.innerHTML = regName;
        tr.appendChild(th);
        var tdTag = document.createElement("td");
        tdTag.classList.add("text-sm-right");
        tr.appendChild(tdTag);
        addTagInput(regName, tdTag);
        var td = document.createElement("td");
        td.id = regName + "-value";
        td.classList.add("text-sm-right");
        tr.appendChild(td);
        document.getElementById("scalar-register-tbody").appendChild(tr);
    }
}
function initiateVGPRsTable() {
    var tbody = document.getElementById("vgpr-tbody");
    initializeVGPRLaneHeaders();
    initiateEXECRow(tbody);
    initiateVCCRow(tbody);
    initiateVGPRRows(tbody);
}
function initializeVGPRLaneHeaders() {
    var headerTr = document.getElementById("vgpr-header-tr");
    for (var i = 0; i < 64; i++) {
        var th = document.createElement("th");
        th.classList.add("text-sm-right");
        th.classList.add("added");
        th.scope = "col";
        th.innerHTML = i.toString();
        headerTr.appendChild(th);
    }
}
function initiateVGPRRows(tbody) {
    for (var i = 0; i < project.data[0]["VGPRs"].length; i++) {
        var regName = "v" + i.toString();
        var tr = document.createElement("tr");
        tr.classList.add("added");
        var th = document.createElement("th");
        th.innerHTML = regName;
        tr.appendChild(th);
        var tdTag = document.createElement("td");
        tdTag.classList.add("text-sm-right");
        addTagInput(regName, tdTag);
        tr.appendChild(tdTag);
        for (var j = 0; j < 64; j++) {
            var td = document.createElement("td");
            td.classList.add("text-sm-right");
            td.id = regName + "-lane" + j.toString() + "-value";
            tr.append(td);
        }
        tbody.appendChild(tr);
    }
}
function addTagInput(regName, tdTag) {
    var tagInput = document.createElement("input");
    tagInput.id = regName + "-tag-input";
    tagInput.classList.add("reg-tag-input");
    tagInput.type = "text";
    tdTag.appendChild(tagInput);
    tagInput.addEventListener("keyup", function (ev) {
        if (ev.keyCode == 13) {
            var tagContent = ev.target.value;
            project.addTag(regName, project.currentPage, tagContent);
        }
    });
}
function initiateVCCRow(tbody) {
    var tr = document.createElement("tr");
    tr.classList.add("added");
    var th = document.createElement("th");
    th.innerHTML = "VCC";
    tr.appendChild(th);
    var tdTag = document.createElement("td");
    tdTag.classList.add("text-sm-right");
    tr.appendChild(tdTag);
    for (var j = 0; j < 64; j++) {
        var td = document.createElement("td");
        td.classList.add("text-sm-right");
        td.id = "VCC-lane" + j.toString() + "-value";
        tr.append(td);
    }
    tbody.appendChild(tr);
}
function initiateEXECRow(tbody) {
    var tr = document.createElement("tr");
    tr.classList.add("added");
    var th = document.createElement("th");
    th.innerHTML = "EXEC";
    tr.appendChild(th);
    var tdTag = document.createElement("td");
    tdTag.classList.add("text-sm-right");
    tr.appendChild(tdTag);
    for (var j = 0; j < 64; j++) {
        var td = document.createElement("td");
        td.classList.add("text-sm-right");
        td.id = "EXEC-lane" + j.toString() + "-value";
        tr.append(td);
    }
    tbody.appendChild(tr);
}
function showPage(data, pageId) {
    document.getElementById("current-page-number").innerHTML = project.currentPage.toString();
    updateValidateButton();
    updateValidationProgressBar();
    showData(data[pageId]);
}
function showData(data) {
    updateValue("pc", "0x" + (data["PCHi"] + data["PCLo"]).toString(16));
    updateValue("inst", data["Inst"]);
    updateValue("scc", data["SCC"]);
    showVCC(data);
    showEXEC(data);
    showSGPRs(data);
    showVGPRs(data);
}
function showVCC(data) {
    var vccLo = data["VCCLo"];
    var vccHi = data["VCCHi"];
    for (var i = 0; i < 64; i++) {
        var bit = 0;
        if (i >= 32) {
            bit = vccHi & (1 << (i - 32));
        }
        else {
            bit = vccLo & (1 << i);
        }
        if (bit) {
            updateValue("VCC-lane" + i.toString(), "1");
        }
        else {
            updateValue("VCC-lane" + i.toString(), "0");
        }
    }
}
function showEXEC(data) {
    var execLo = data["EXECLo"];
    var execHi = data["EXECHi"];
    for (var i = 0; i < 64; i++) {
        var bit = 0;
        if (i >= 32) {
            bit = execHi & (1 << (i - 32));
        }
        else {
            bit = execLo & (1 << i);
        }
        if (bit) {
            updateValue("EXEC-lane" + i.toString(), "1");
        }
        else {
            updateValue("EXEC-lane" + i.toString(), "0");
        }
    }
}
function showSGPRs(data) {
    for (var i = 0; i < data["SGPRs"].length; i++) {
        updateValue("s" + i.toString(), data["SGPRs"][i].toString(16));
        showTags("s" + i.toString());
    }
}
function showVGPRs(data) {
    for (var i = 0; i < data["VGPRs"].length; i++) {
        for (var j = 0; j < data["VGPRs"][i].length; j++) {
            updateValue("v" + i.toString() + "-lane" + j.toString(), data["VGPRs"][i][j].toString(16));
        }
    }
}
function updateValue(content, value) {
    var element = document.getElementById(content + "-value");
    if (element.innerHTML != value) {
        element.classList.add("changed");
    }
    else {
        element.classList.remove("changed");
    }
    element.innerHTML = value;
}
function showTags(content) {
    var tagInput = (document.getElementById(content + "-tag-input"));
    var tag = project.searchTag(content, project.currentPage);
    tagInput.value = tag;
}
