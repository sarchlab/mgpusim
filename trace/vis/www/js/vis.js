(function (vis) {
    'strict mode';

    $(document).ready(function () {
        resize();
        $(window).resize(debouncer(function () {
            resize();
        }));
    });

    function debouncer( func , timeout ) {
        var timeoutID , timeout = timeout || 200;
        return function () {
            var scope = this , args = arguments;
            clearTimeout( timeoutID );
            timeoutID = setTimeout( function () {
                func.apply( scope , Array.prototype.slice.call( args ) );
            } , timeout );
        };
    }

    function resize() {
        let windowHeight = $(window).height();
        let windowWidth = $(window).width();

        $('#full-screen')
            .height(windowHeight)
            .width(windowWidth);

        $('#minimap')
            .height(200)
            .width(windowWidth);

        $('#pipeline-figure')
            .height(windowHeight - 200)
            .width(windowWidth);

        vis.minimap.loadAndDraw(vis.trace.loadAndDraw.bind(vis.trace));
    }
})(window.vis = window.vis || {});