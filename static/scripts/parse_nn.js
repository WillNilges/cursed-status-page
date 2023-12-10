window.onload = function() {
    // Define a regular expression to match the "nn713" format
    var regex = /nn(\d+)/g;
    var capRegex = /NN(\d+)/g;

    // Get the HTML content of the entire page
    var pageContent = document.body.innerHTML;

    // Replace all occurrences of "nn713" with the link
    var replacedContent = pageContent.replace(regex, parseNode);
    replacedContent = replacedContent.replace(capRegex, parseNode);

    // Update the HTML content of the page with the modified content
    document.body.innerHTML = replacedContent;
};

function parseNode(match, number) {
    return '<a target="_blank" href="https://www.nycmesh.net/map/nodes/' + number + '">' + match + '</a>';
}
