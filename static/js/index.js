const frmCreateRoute = document.getElementById('frm-create-route')
const intRouteFrom = document.getElementById('int-route-from')
const intRouteTo = document.getElementById('int-route-to')
const sltRouteType = document.getElementById('slt-route-type')

const btnCreateRoute = document.getElementById('btn-create-route')
btnCreateRoute.addEventListener('click', () => {
    intRouteFrom.value = intRouteFrom.value.trim()
    intRouteTo.value = intRouteTo.value.trim()

    if (!frmCreateRoute.reportValidity()) {
        console.log()
        return
    }

    const from = intRouteFrom.value
    const to = intRouteTo.value
    const type = sltRouteType.value

    fetch('/api/v1/routes', {
        method: 'POST',
        body: JSON.stringify({ from, to, type })
    }).then(() => document.location.reload())
})

const btnsDeleteRoute = document.getElementsByClassName('btn-delete-route')
for (let btn of btnsDeleteRoute) {
    btn.addEventListener('click', e => {
        const from = e.target.dataset.from

        fetch('/api/v1/routes', {
            method: 'DELETE',
            body: JSON.stringify({ from })
        }).then(() => document.location.reload())
    })
}
