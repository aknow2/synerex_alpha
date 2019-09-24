import React from 'react';
import { GridCellLayer } from 'deck.gl';
import {
  Container, connectToHarmowareVis, HarmoVisLayers, MovesLayer, LoadingIcon, FpsDisplay,
} from 'harmoware-vis';

import * as io from 'socket.io-client';
import rainData from './rain';
import Controller from '../components/controller.jsx';


const MAPBOX_TOKEN = 'pk.eyJ1IjoidG1rbnltIiwiYSI6ImNrMDNuYTA3NTJ4YzUzbnV0ZGxycjRtMmgifQ.B6rGu500hfC4hbhLei9vMA'; // Acquire Mapbox accesstoken
console.log('TOKEN');
console.log(MAPBOX_TOKEN);

class App extends Container {
  constructor(props) {
    super(props);
    const { setSecPerHour, setLeading, setTrailing } = props.actions;
    setSecPerHour(3600);
    setLeading(3);
    setTrailing(3);
    const socket = io();
    this.state = {
      moveDataVisible: true,
      moveOptionVisible: false,
      depotOptionVisible: false,
      heatmapVisible: false,
      optionChange: false,
      popup: [0, 0, ''],
      rainfallData: []
    };

    // for receiving event info.
    socket.on('connect', () => { console.log('Socket.IO connected!'); });
    socket.on('event', this.getEvent);
    socket.on('notify_new_rainfall_data', this.recivedRainfallData)
    socket.on('disconnect', () => { console.log('Socket.IO disconnected!'); });
  }

  recivedRainfallData = rainfallJsonData => {
    const rainfallData = JSON.parse(rainfallJsonData)
    console.log(rainfallData)
    this.setState({
      rainfallData
    })
  }

  getEvent = (socketData) => {
    const { actions, movesbase } = this.props;
    const recivedData = JSON.parse(socketData);

    if (Object.keys(recivedData).length === 0) {
      // if object is empty
      return;
    }

    const {
      mtype, id, lat, lon, angle, speed,
    } = recivedData;
    // console.log("dt:",mtype,id,time,lat,lon,angle,speed, socketData);
    const time = Date.now() / 1000; // set time as now. (If data have time, ..)
    let hit = false;
    const movesbasedata = [...movesbase]; // why copy !?
    const setMovesbase = [];

    for (let i = 0, lengthi = movesbasedata.length; i < lengthi; i += 1) {
      // let setMovedata = Object.assign({}, movesbasedata[i]);
      const setMovedata = movesbasedata[i];
      if (mtype === setMovedata.mtype && id === setMovedata.id) {
        hit = true;
        setMovedata.arrivaltime = time;
        setMovedata.operation.push({
          elapsedtime: time,
          position: [lon, lat, 0],
          angle,
          speed,
        });
      }
      setMovesbase.push(setMovedata);
    }
    if (!hit) {
      setMovesbase.push({
        mtype,
        id,
        departuretime: time,
        arrivaltime: time,
        operation: [{
          elapsedtime: time,
          position: [lon, lat, 0],
          angle,
          speed,
        }],
      });
    }
    actions.updateMovesBase(setMovesbase);
  }

  deleteMovebase = (maxKeepSecond) => {
    const {
      actions, animatePause, movesbase, settime,
    } = this.props;
    const movesbasedata = [...movesbase];
    const setMovesbase = [];
    let dataModify = false;
    const compareTime = settime - maxKeepSecond;
    for (let i = 0, lengthi = movesbasedata.length; i < lengthi; i += 1) {
      const { departuretime: propsdeparturetime, operation: propsoperation } = movesbasedata[i];
      let departuretime = propsdeparturetime;
      let startIndex = propsoperation.length;
      for (let j = 0, lengthj = propsoperation.length; j < lengthj; j += 1) {
        if (propsoperation[j].elapsedtime > compareTime) {
          startIndex = j;
          departuretime = propsoperation[j].elapsedtime;
          break;
        }
      }
      if (startIndex === 0) {
        setMovesbase.push({ ...movesbasedata[i] });
      } else
      if (startIndex < propsoperation.length) {
        setMovesbase
          .push({
            ...movesbasedata[i],
            operation: propsoperation.slice(startIndex),
            departuretime,
          });
        dataModify = true;
      } else {
        dataModify = true;
      }
    }
    if (dataModify) {
      if (!animatePause) {
        actions.setAnimatePause(true);
      }
      actions.updateMovesBase(setMovesbase);
      if (!animatePause) {
        actions.setAnimatePause(false);
      }
    }
  }

  getMoveDataChecked = (e) => {
    this.setState({ moveDataVisible: e.target.checked });
  }

  getMoveOptionChecked = (e) => {
    this.setState({ moveOptionVisible: e.target.checked });
  }

  getDepotOptionChecked = (e) => {
    this.setState({ depotOptionVisible: e.target.checked });
  }

  getOptionChangeChecked = (e) => {
    this.setState({ optionChange: e.target.checked });
  }

  getMoveLayer = () => {
    const {
      actions, clickedObject, viewport,
      routePaths, lightSettings, movesbase, movedData,
    } = this.props;
    const onHover = (el) => {
      if (el && el.object) {
        let disptext = '';
        const objctlist = Object.entries(el.object);
        for (let i = 0, lengthi = objctlist.length; i < lengthi; i += 1) {
          const strvalue = objctlist[i][1].toString();
          disptext += i > 0 ? '\n' : '';
          disptext += (`${objctlist[i][0]}: ${strvalue}`);
        }
        this.setState({ popup: [el.x, el.y, disptext] });
      } else {
        this.setState({ popup: [0, 0, ''] });
      }
    };
    return this.state.moveDataVisible && this.props.movedData.length > 0
      ? new MovesLayer({
        viewport,
        routePaths,
        movesbase,
        movedData,
        clickedObject,
        actions,
        lightSettings,
        visible: this.state.moveDataVisible,
        optionVisible: this.state.moveOptionVisible,
        optionChange: this.state.optionChange,
        onHover,
      }) : null;
  }

  getRainFallLayer = () => {
    const { lightSettings } = this.props;
    const { rainfallData } = this.state
    return new GridCellLayer({
      data: rainfallData,
      getElevation: (x) => Math.pow(x.elevation, 2) || 0,
      getColor: (x) => x.color,
      opacity: 0.2,
      cellSize: 100,
      elevationScale: 30,
      lightSettings,
      pickable: true,
    });
  }

  render() {
    const { props } = this;
    const {
      actions, viewport, loading,
    } = props;
    // const { movesFileName } = inputFileName;

    return (
      <div>
        <Controller
          {...props}
          deleteMovebase={this.deleteMovebase}
          getMoveDataChecked={this.getMoveDataChecked}
          getMoveOptionChecked={this.getMoveOptionChecked}
          getDepotOptionChecked={this.getDepotOptionChecked}
          getOptionChangeChecked={this.getOptionChangeChecked}
        />
        <div className="harmovis_area">
          <HarmoVisLayers
            viewport={viewport}
            actions={actions}
            mapboxApiAccessToken={MAPBOX_TOKEN}
            layers={[
              this.getMoveLayer(),
              this.getRainFallLayer(),
            ]}
          />
        </div>
        <svg width={viewport.width} height={viewport.height} className="harmovis_overlay">
          <g fill="white" fontSize="12">
            {this.state.popup[2].length > 0
              ? this.state.popup[2].split('\n').map((value, index) => (
                <text
                  x={this.state.popup[0] + 10}
                  y={this.state.popup[1] + (index * 12)}
                  key={index.toString()}
                >
                  {value}
                </text>
              )) : null}
          </g>
        </svg>
        <LoadingIcon loading={loading} />
        <FpsDisplay />
      </div>
    );
  }
}
export default connectToHarmowareVis(App);
