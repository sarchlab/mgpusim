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
        let data = this.data;
        let width = window.innerWidth;
        let height = 200;

        let svg = d3.select('#minimap').selectAll('svg')
            .attr('height', height)
            .attr('width', width);

        let startTime = data[0].start_time;
        let endTime = data[data.length - 1].end_time;
        this.xScale = d3.scaleLinear()
            .domain([startTime, endTime])
            .range([0, width]);
        let widthScale = d3.scaleLinear()
            .domain([0, endTime - startTime])
            .range([0, width]);
        let xAxis = d3.axisTop(this.xScale);

        let highestCount = d3.max(data, function(d){ return d.count; });
        let verticalScale = d3.scaleLinear()
            .domain([0, highestCount])
            .range([height, 0]);
        let yAxis = d3.axisRight(verticalScale);


        let minimapBars = svg.select('.main-area')
            .selectAll('rect')
            .data(data);

        let minimapBarsEnter = minimapBars.enter()
            .append('rect')
            .style('fill', '#ffd385');

        minimapBars.merge(minimapBarsEnter)
            .transition()
            .attr('x', function (d) {
                return vis.minimap.xScale(d.start_time);
            })
            .attr('y', function (d) {
                return verticalScale(d.count);
            })
            .attr('width', function (d) {
                return widthScale(d.end_time - d.start_time);
            })
            .attr('height', function (d) {
                return height - verticalScale(d.count);
            });

        // Brush
        let brush = d3.brushX()
            .extent([[0, 0], [width, height]])
            .on('brush end', this.brushed.bind(this));
        svg.select('.brush')
            .call(brush)
            .call(brush.move, [width / 10, width / 5]);


        // Axises
        svg.selectAll('.x-axis')
            .attr("transform", "translate(0, 200)")
            .call(xAxis);
        svg.selectAll('.y-axis')
            .call(yAxis);
    }

    brushed() {
        let s = d3.event.selection;
        let timeRange = s.map(this.xScale.invert, this.xScale);
        this.onRangeSelected(timeRange);
    }
};

vis.minimap = new Minimap();

}(window.vis = window.vis || {}));