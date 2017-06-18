(function (vis) {

class Minimap {
    constructor() {
        this.data = undefined;
        this.xScale = undefined;
        this.onRangeSelected = undefined;
    }

    loadAndDraw(onRangeSelected) {
        this.onRangeSelected = onRangeSelected;
        this.loadData(this.render.bind(this));
    }

    loadData(done) {
        $.ajax({
            url: 'minimap',
            method: 'GET',
            data: { num_samples: Math.floor(window.innerWidth / 2) },
            dataType: "json"
        }).done(function (data) {
            this.data = data;
            done();
       }.bind(this));
    }

    render() {
        this.width = window.innerWidth;
        this.height = 200;

        this.svg = d3.select('#minimap').selectAll('svg')
            .attr('height', this.height)
            .attr('width', this.width);

        this.defineScales();
        this.drawMinibars();
        this.applyBrush();
        this.applyAxises();
    }

    defineScales() {
        let startTime = this.data[0].start_time;
        let endTime = this.data[this.data.length - 1].end_time;
        this.xScale = d3.scaleLinear()
            .domain([startTime, endTime])
            .range([0, this.width]);
        this.widthScale = d3.scaleLinear()
            .domain([0, endTime - startTime])
            .range([0, this.width]);

        let highestCount = d3.max(this.data, function(d){ return d.count; });
        this.yScale = d3.scaleLinear()
            .domain([0, highestCount])
            .range([this.height, 0]);
    }

    drawMinibars() {
        let minimapBars = this.svg.select('.main-area')
            .selectAll('rect')
            .data(this.data);

        let minimapBarsEnter = minimapBars.enter()
            .append('rect')
            .style('fill', '#ffd385');

        minimapBars.merge(minimapBarsEnter)
            .transition()
            .attr('x', function (d) {
                return this.xScale(d.start_time);
            }.bind(this))
            .attr('y', function (d) {
                return this.yScale(d.count);
            }.bind(this))
            .attr('width', function (d) {
                return this.widthScale(d.end_time - d.start_time);
            }.bind(this))
            .attr('height', function (d) {
                return this.height - this.yScale(d.count);
            }.bind(this));


    }

    applyAxises() {
        let xAxis = d3.axisTop(this.xScale);
        this.svg.selectAll('.x-axis')
            .attr("transform", "translate(0, 200)")
            .call(xAxis);
        let yAxis = d3.axisRight(this.yScale);
        this.svg.selectAll('.y-axis')
            .call(yAxis);
    }

    applyBrush() {
        let brush = d3.brushX()
            .extent([[0, 0], [this.width, this.height]])
            .on('brush end', this.brushed.bind(this));
        this.svg.select('.brush')
            .call(brush)
            .call(brush.move, [this.width / 10, this.width / 8]);
    }

    brushed() {
        let s = d3.event.selection;
        let timeRange = s.map(this.xScale.invert, this.xScale);
        this.onRangeSelected(timeRange);
    }
};

vis.minimap = new Minimap();

}(window.vis = window.vis || {}));